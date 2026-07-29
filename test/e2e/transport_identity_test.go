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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
		var (
			publisher        *e2eutils.KafkaEventPublisher
			spoofMigrationNS string
		)

		BeforeEach(func() {
			spoofMigrationNS = fmt.Sprintf("%s-%d", spoofMigrationNSPrefix, time.Now().UnixNano())

			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, globalHubClient, constants.GHDefaultNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from global hub transport-config secret")
		})

		AfterEach(func() {
			deleteNamespaceAndWait(targetHubClient, spoofMigrationNS)
		})

		It("should drop migration deploying events when no in-flight migration is recorded on the target hub", func() {
			migrationID := fmt.Sprintf("%s-no-state-%d", spoofMigrationID, time.Now().UnixNano())
			evt := migrationDeployingEvent(
				sourceHubName,
				targetHubName,
				spoofMigrationNS,
				migrationID,
			)
			Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
				"expected migration event to publish to Kafka for rejection testing")

			Consistently(func() error {
				ns := &corev1.Namespace{}
				err := targetHubClient.Get(ctx, types.NamespacedName{Name: spoofMigrationNS}, ns)
				if err == nil {
					return fmt.Errorf("namespace %q must not be created without in-flight migration state", spoofMigrationNS)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"migration deploying event must not create resources without in-flight migration state")
		})

		It("should drop migration deploying events from an untrusted source hub", func() {
			migrationID := fmt.Sprintf("%s-untrusted-%d", spoofMigrationID, time.Now().UnixNano())
			seedClusterName := fmt.Sprintf("e2e-transport-id-seed-%d", time.Now().UnixNano())
			seedMSAName := fmt.Sprintf("e2e-transport-id-msa-%d", time.Now().UnixNano())

			seedInFlightMigrationState(
				publisher,
				sourceHubName,
				targetHubName,
				migrationID,
				seedMSAName,
				seedClusterName,
			)

			probeNS := fmt.Sprintf("%s-probe-%d", spoofMigrationNSPrefix, time.Now().UnixNano())
			waitForTrustedMigrationDeploy(
				publisher,
				targetHubClient,
				sourceHubName,
				targetHubName,
				probeNS,
				migrationID,
			)
			deleteNamespaceAndWait(targetHubClient, probeNS)

			evt := migrationDeployingEvent(
				spoofMigrationSource,
				targetHubName,
				spoofMigrationNS,
				migrationID,
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
				"spoofed migration deploying event must not create resources when in-flight migration is registered for a different source hub")
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

func migrationDeployingEvent(sourceHub, targetHub, resourceName, migrationID string) cloudevents.Event {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: resourceName}}
	unstructuredNS, err := runtime.DefaultUnstructuredConverter.ToUnstructured(ns)
	Expect(err).NotTo(HaveOccurred(), "expected namespace object conversion for migration bundle")
	obj := unstructured.Unstructured{Object: unstructuredNS}
	obj.SetKind("Namespace")
	obj.SetAPIVersion("v1")

	bundle := migrationbundle.MigrationResourceBundle{
		TotalClusters: 1,
		MigrationClusterResources: []migrationbundle.MigrationClusterResource{
			{
				ClusterName:  resourceName,
				ResourceList: []unstructured.Unstructured{obj},
			},
		},
	}

	return pkgutils.ToMigrationEvent(
		string(enum.ManagedClusterMigrationType),
		sourceHub,
		targetHub,
		migrationID,
		migrationv1alpha1.PhaseDeploying,
		10*time.Minute,
		bundle,
	)
}

func migrationValidatingEvent(
	sourceHub, targetHub, migrationID, managedServiceAccountName, clusterName string,
) cloudevents.Event {
	bundle := migrationbundle.MigrationTargetBundle{
		FromHub:                   sourceHub,
		ManagedServiceAccountName: managedServiceAccountName,
		ManagedClusters:           []string{clusterName},
	}

	return pkgutils.ToMigrationEvent(
		string(enum.ManagedClusterMigrationType),
		constants.CloudEventGlobalHubClusterName,
		targetHub,
		migrationID,
		migrationv1alpha1.PhaseValidating,
		10*time.Minute,
		bundle,
	)
}

func seedInFlightMigrationState(
	publisher *e2eutils.KafkaEventPublisher,
	sourceHub, targetHub, migrationID, managedServiceAccountName, clusterName string,
) {
	evt := migrationValidatingEvent(sourceHub, targetHub, migrationID, managedServiceAccountName, clusterName)
	Expect(publisher.SendToTopic(ctx, publisher.SpecTopic(), evt)).To(Succeed(),
		"expected validating migration event to seed in-flight migration state on target hub agent")
}

func waitForTrustedMigrationDeploy(
	publisher *e2eutils.KafkaEventPublisher,
	targetClient client.Client,
	sourceHub, targetHub, probeNamespace, migrationID string,
) {
	Eventually(func() error {
		evt := migrationDeployingEvent(sourceHub, targetHub, probeNamespace, migrationID)
		if err := publisher.SendToTopic(ctx, publisher.SpecTopic(), evt); err != nil {
			return fmt.Errorf("publish trusted migration deploying probe event: %w", err)
		}

		ns := &corev1.Namespace{}
		err := targetClient.Get(ctx, types.NamespacedName{Name: probeNamespace}, ns)
		if err != nil {
			return fmt.Errorf("wait for trusted migration deploy to create namespace %q: %w", probeNamespace, err)
		}
		return nil
	}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
		"trusted migration deploying event must succeed once in-flight migration state is registered for the source hub")
}

func deleteNamespaceAndWait(targetClient client.Client, namespaceName string) {
	err := targetClient.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespaceName}})
	if err != nil && !apierrors.IsNotFound(err) {
		Expect(err).NotTo(HaveOccurred(), "expected to delete namespace %q during test cleanup", namespaceName)
	}

	Eventually(func() bool {
		err := targetClient.Get(ctx, types.NamespacedName{Name: namespaceName}, &corev1.Namespace{})
		return client.IgnoreNotFound(err) == nil && err != nil
	}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
		"expected namespace %q to be fully removed before next test", namespaceName)
}
