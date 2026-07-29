// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"errors"
	"fmt"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	pkgutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
	e2eutils "github.com/stolostron/multicluster-global-hub/test/e2e/utils"
)

const (
	spoofVictimHubName     = "e2e-spoof-victim-hub"
	spoofMigrationSource   = "e2e-spoof-migration-source"
)

var _ = Describe("Transport Identity E2E", Label("e2e-test-transport-identity"), Ordered, func() {
	var (
		sourceHubName   string
		targetHubName   string
		targetHubClient client.Client
	)

	BeforeAll(func() {
		Expect(len(managedHubNames)).To(BeNumerically(">=", 2),
			"transport identity e2e requires two regional hubs (source and target)")
		sourceHubName = managedHubNames[0]
		targetHubName = managedHubNames[1]

		var err error
		targetHubClient, err = testClients.RuntimeClient(targetHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected target hub %q kubeconfig to be valid", targetHubName)
	})

	Context("ACM-34442 Phase 1 - manager status identity from Kafka topic", func() {
		var publisher *e2eutils.KafkaEventPublisher

		BeforeEach(func() {
			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, globalHubClient, constants.GHDefaultNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from global hub transport-config secret")
		})

		AfterEach(func() {
			for _, hub := range []string{spoofVictimHubName, sourceHubName} {
				Expect(database.GetGorm().Exec(
					`DELETE FROM status.leaf_hub_heartbeats WHERE leaf_hub_name = $1`,
					hub,
				).Error).To(Succeed(), "expected heartbeat cleanup for hub %q between tests", hub)
			}
		})

		It("should drop status events when CloudEvent source does not match the Kafka status topic hub", func() {
			statusTopic := publisher.StatusTopic(sourceHubName)
			evt := statusCloudEvent(
				statusTopic,
				spoofVictimHubName,
				string(enum.HubClusterHeartbeatType),
				generic.GenericObjectBundle{},
			)

			Expect(publisher.SendToTopic(ctx, statusTopic, *evt)).To(Succeed(),
				"expected spoofed status event to publish to Kafka for rejection testing")

			Consistently(func() error {
				for _, hub := range []string{sourceHubName, spoofVictimHubName} {
					var heartbeat models.LeafHubHeartbeat
					err := database.GetGorm().Where("leaf_hub_name = ?", hub).First(&heartbeat).Error
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					if err != nil {
						return fmt.Errorf("query heartbeat for hub %q: %w", hub, err)
					}
					if hub == spoofVictimHubName {
						return fmt.Errorf("spoofed status event must not create heartbeat for hub %q", hub)
					}
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"spoofed status heartbeat must not be persisted when CloudEvent source mismatches topic hub")
		})

		It("should accept status events when CloudEvent source matches the Kafka status topic hub", func() {
			statusTopic := publisher.StatusTopic(sourceHubName)
			evt := statusCloudEvent(
				statusTopic,
				sourceHubName,
				string(enum.HubClusterHeartbeatType),
				generic.GenericObjectBundle{},
			)

			Expect(publisher.SendToTopic(ctx, statusTopic, *evt)).To(Succeed(),
				"expected trusted status event to publish to Kafka")

			Eventually(func() error {
				var heartbeat models.LeafHubHeartbeat
				err := database.GetGorm().Where("leaf_hub_name = ?", sourceHubName).First(&heartbeat).Error
				if err != nil {
					return fmt.Errorf("expected heartbeat row for trusted hub %q: %w", sourceHubName, err)
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"trusted status heartbeat must be persisted when CloudEvent source matches topic hub")
		})
	})

	Context("ACM-34442 Phase 2 - agent spec source validation", func() {
		var publisher *e2eutils.KafkaEventPublisher

		BeforeEach(func() {
			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, globalHubClient, constants.GHDefaultNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from global hub transport-config secret")
		})

		It("should drop migration spec events from an untrusted source hub", func() {
			msaName := fmt.Sprintf("e2e-spoof-msa-%d", time.Now().UnixNano())
			payload := bundleevent.ManagedClusterMigrationToEvent{
				ManagedServiceAccountName:             msaName,
				ManagedServiceAccountInstallNamespace: constants.GHDefaultNamespace,
			}
			evt := pkgutils.ToCloudEvent(
				constants.CloudEventTypeMigrationTo,
				spoofMigrationSource,
				targetHubName,
				payload,
			)
			Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
				"expected spoofed migration spec event to publish to Kafka for rejection testing")

			Consistently(func() error {
				cr := &rbacv1.ClusterRole{}
				err := targetHubClient.Get(ctx, types.NamespacedName{
					Name: fmt.Sprintf("multicluster-global-hub-migration:%s", msaName),
				}, cr)
				if err == nil {
					return fmt.Errorf("migration clusterrole must not be created from untrusted source %q", spoofMigrationSource)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"GH 1.4 agent must reject spec events whose CloudEvent source is not global-hub")
		})

		It("should drop generic spec events from an untrusted source hub", func() {
			evt := pkgutils.ToCloudEvent(
				constants.GenericSpecMsgKey,
				spoofMigrationSource,
				targetHubName,
				generic.GenericObjectBundle{},
			)
			Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
				"expected spoofed generic spec event to publish to Kafka for rejection testing")

			Consistently(func() error {
				cr := &rbacv1.ClusterRole{}
				err := targetHubClient.Get(ctx, types.NamespacedName{
					Name: fmt.Sprintf("multicluster-global-hub-migration:%s", spoofMigrationSource),
				}, cr)
				if err == nil {
					return fmt.Errorf("resources must not be created from untrusted generic spec source %q", spoofMigrationSource)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"spoofed generic spec events must be dropped before reaching syncers on GH 1.4")
		})
	})
})

func statusCloudEvent(kafkaTopic, source, eventType string, data interface{}) *cloudevents.Event {
	version := eventversion.NewVersion()
	version.Incr()
	evt := cloudevents.NewEvent()
	evt.SetSource(source)
	evt.SetType(eventType)
	evt.SetExtension(eventversion.ExtVersion, version.String())
	_ = evt.SetData(cloudevents.ApplicationJSON, data)
	evt.SetExtension(kafka_confluent.KafkaTopicKey, kafkaTopic)
	return &evt
}
