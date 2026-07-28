// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"context"
	"errors"
	"fmt"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
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
	spoofMigrationID       = "e2e-spoof-migration"
	spoofMigrationNSPrefix = "e2e-spoof-migration-ns"
)

var _ = Describe("Transport Identity E2E", Label("e2e-test-transport-identity"), Ordered, func() {
	var (
		sourceHubName   string
		targetHubName   string
		sourceHubClient client.Client
		targetHubClient client.Client
	)

	BeforeAll(func() {
		Expect(len(managedHubNames)).To(BeNumerically(">=", 2),
			"transport identity e2e requires two regional hubs (source and target)")
		sourceHubName = managedHubNames[0]
		targetHubName = managedHubNames[1]

		var err error
		sourceHubClient, err = testClients.RuntimeClient(sourceHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected source hub %q kubeconfig to be valid", sourceHubName)
		targetHubClient, err = testClients.RuntimeClient(targetHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected target hub %q kubeconfig to be valid", targetHubName)
	})

	Context("ACM-34442 Phase 1 - manager status identity from Kafka topic", func() {
		var publisher *e2eutils.KafkaEventPublisher

		BeforeEach(func() {
			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, sourceHubClient, constants.GHAgentNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from source hub transport secret")
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
		var (
			publisher        *e2eutils.KafkaEventPublisher
			spoofMigrationNS string
			testClusterName  string
		)

		BeforeEach(func() {
			spoofMigrationNS = fmt.Sprintf("%s-%d", spoofMigrationNSPrefix, time.Now().UnixNano())
			Expect(len(managedClusterNames)).To(BeNumerically(">=", 1),
				"migration source validation e2e requires at least one managed cluster")
			testClusterName = managedClusterNames[0]

			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, sourceHubClient, constants.GHAgentNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from source hub transport secret")
		})

		AfterEach(func() {
			_ = targetHubClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: spoofMigrationNS}})
			_ = targetHubClient.Delete(ctx, &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{Name: spoofMigrationID, Namespace: constants.GHDefaultNamespace},
			})
			Eventually(func() bool {
				err := targetHubClient.Get(ctx, types.NamespacedName{
					Name: spoofMigrationID, Namespace: constants.GHDefaultNamespace,
				}, &migrationv1alpha1.ManagedClusterMigration{})
				return client.IgnoreNotFound(err) == nil && err != nil
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"expected migration CR to be fully removed before next test")
			Eventually(func() bool {
				err := targetHubClient.Get(ctx, types.NamespacedName{Name: spoofMigrationNS}, &corev1.Namespace{})
				return client.IgnoreNotFound(err) == nil && err != nil
			}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
				"expected spoof migration namespace to be fully removed before next test")
		})

		It("should drop migration deploying events from an untrusted source hub", func() {
			ensureDeployingMigrationCR(ctx, targetHubClient, sourceHubName, targetHubName, testClusterName)

			evt := migrationDeployingEvent(
				spoofMigrationSource,
				targetHubName,
				spoofMigrationNS,
				testClusterName,
			)
			Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
				"expected spoofed migration event to publish to Kafka for rejection testing")

			Consistently(func() error {
				ns := &corev1.Namespace{}
				err := targetHubClient.Get(ctx, types.NamespacedName{Name: spoofMigrationNS}, ns)
				if err == nil {
					return fmt.Errorf("namespace %q must not be created from spoofed migration source", spoofMigrationNS)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"spoofed migration deploying event must not create resources on target hub")
		})

		It("should drop migration deploying events when no in-flight migration is recorded on the target hub", func() {
			evt := migrationDeployingEvent(
				sourceHubName,
				targetHubName,
				spoofMigrationNS,
				testClusterName,
			)
			Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
				"expected migration event to publish to Kafka for rejection testing")

			Consistently(func() error {
				ns := &corev1.Namespace{}
				err := targetHubClient.Get(ctx, types.NamespacedName{Name: spoofMigrationNS}, ns)
				if err == nil {
					return fmt.Errorf("namespace %q must not be created without in-flight migration CR", spoofMigrationNS)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"migration deploying event must not create resources without in-flight migration CR")
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

func ensureDeployingMigrationCR(
	ctx context.Context,
	hubClient client.Client,
	fromHub, toHub, clusterName string,
) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: constants.GHDefaultNamespace}}
	Expect(client.IgnoreAlreadyExists(hubClient.Create(ctx, ns))).To(Succeed(),
		"expected migration namespace %q to exist on target hub", constants.GHDefaultNamespace)

	migrationCR := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spoofMigrationID,
			Namespace: constants.GHDefaultNamespace,
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From:                    fromHub,
			To:                      toHub,
			IncludedManagedClusters: []string{clusterName},
		},
	}
	Expect(client.IgnoreAlreadyExists(hubClient.Create(ctx, migrationCR))).To(Succeed(),
		"expected in-flight migration CR %q to be registered on target hub", spoofMigrationID)

	current := &migrationv1alpha1.ManagedClusterMigration{}
	Expect(hubClient.Get(ctx, types.NamespacedName{
		Name: spoofMigrationID, Namespace: constants.GHDefaultNamespace,
	}, current)).To(Succeed(), "expected migration CR %q to be readable before status update", spoofMigrationID)
	current.Status.Phase = migrationv1alpha1.PhaseDeploying
	Expect(hubClient.Status().Update(ctx, current)).To(Succeed(),
		"expected migration CR %q to enter deploying phase for source validation", spoofMigrationID)
}

func migrationDeployingEvent(sourceHub, targetHub, namespaceName, clusterName string) cloudevents.Event {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}}
	unstructuredNS, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ns)
	Expect(err).NotTo(HaveOccurred(), "expected namespace object conversion for migration bundle")
	obj := unstructured.Unstructured{Object: unstructuredNS}
	obj.SetKind("Namespace")
	obj.SetAPIVersion("v1")

	bundle := migrationbundle.MigrationResourceBundle{
		TotalClusters: 1,
		MigrationClusterResources: []migrationbundle.MigrationClusterResource{
			{
				ClusterName:  clusterName,
				ResourceList: []unstructured.Unstructured{obj},
			},
		},
	}

	return pkgutils.ToMigrationEvent(
		string(enum.ManagedClusterMigrationType),
		sourceHub,
		targetHub,
		spoofMigrationID,
		migrationv1alpha1.PhaseDeploying,
		10*time.Minute,
		bundle,
	)
}
