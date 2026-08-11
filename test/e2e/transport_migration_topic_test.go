// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package tests

import (
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorconfig "github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	pkgutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
	e2eutils "github.com/stolostron/multicluster-global-hub/test/e2e/utils"
)

var _ = Describe("Transport Migration Topic E2E", Label("e2e-test-transport-migration-topic"), Ordered, func() {
	var (
		sourceHubName   string
		targetHubName   string
		sourceHubClient client.Client
		targetHubClient client.Client
		migrationTopic  string
	)

	BeforeAll(func() {
		Expect(len(managedHubNames)).To(BeNumerically(">=", 2),
			"transport migration topic e2e requires two regional hubs (source and target)")
		sourceHubName = managedHubNames[0]
		targetHubName = managedHubNames[1]
		// Operator in-process topic config is not initialized in the e2e test binary.
		migrationTopic = operatorconfig.DEFAULT_MIGRATION_TOPIC

		var err error
		sourceHubClient, err = testClients.RuntimeClient(sourceHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected source hub %q kubeconfig to be valid", sourceHubName)
		targetHubClient, err = testClients.RuntimeClient(targetHubName, agentScheme)
		Expect(err).NotTo(HaveOccurred(), "expected target hub %q kubeconfig to be valid", targetHubName)
	})

	Context("ACM-34442 Phase 3 - dedicated gh-migration topic", func() {
		It("should provision the gh-migration KafkaTopic in the global hub namespace", func() {
			topic := &kafkav1beta2.KafkaTopic{}
			Eventually(func() error {
				return globalHubClient.Get(ctx, types.NamespacedName{
					Name:      migrationTopic,
					Namespace: testOptions.GlobalHub.Namespace,
				}, topic)
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"expected gh-migration KafkaTopic to be provisioned in the global hub namespace")
			Expect(topic.Spec.Partitions).NotTo(BeNil(),
				"gh-migration topic must define partitions in spec")
			Expect(int(*topic.Spec.Partitions)).To(BeNumerically(">", 0),
				"gh-migration topic must have at least one partition")
		})

		It("should include the migration topic in managed hub transport credentials", func() {
			Eventually(func() error {
				secret := &corev1.Secret{}
				if err := sourceHubClient.Get(ctx, types.NamespacedName{
					Name:      constants.GHTransportConfigSecret,
					Namespace: constants.GHAgentNamespace,
				}, secret); err != nil {
					return fmt.Errorf("get transport secret on managed hub agent namespace: %w", err)
				}

				kafkaConfig, err := pkgutils.GetKafkaCredentialBySecret(secret, sourceHubClient)
				if err != nil {
					return fmt.Errorf("parse transport secret kafka credentials: %w", err)
				}
				if kafkaConfig.MigrationTopic != migrationTopic {
					return fmt.Errorf("managed hub transport credentials migration topic = %q, want %q",
						kafkaConfig.MigrationTopic, migrationTopic)
				}
				return nil
			}, 2*time.Minute, 5*time.Second).Should(Succeed(),
				"managed hub transport credentials must include the gh-migration topic")
		})
	})

	Context("Migration deploying over gh-migration", func() {
		var (
			publisher        *e2eutils.KafkaEventPublisher
			trustedMigration string
			spoofMigrationNS string
		)

		BeforeEach(func() {
			trustedMigration = fmt.Sprintf("%s-trusted-%d", spoofMigrationNSPrefix, time.Now().UnixNano())
			spoofMigrationNS = fmt.Sprintf("%s-spoof-%d", spoofMigrationNSPrefix, time.Now().UnixNano())

			var err error
			publisher, err = e2eutils.NewKafkaEventPublisher(ctx, globalHubClient, constants.GHDefaultNamespace)
			Expect(err).NotTo(HaveOccurred(), "expected Kafka publisher from global hub transport-config secret")
		})

		AfterEach(func() {
			deleteNamespaceAndWait(targetHubClient, trustedMigration)
			deleteNamespaceAndWait(targetHubClient, spoofMigrationNS)
		})

		It("should apply deploying migration resources received on gh-migration from the registered source hub", func() {
			migrationID := fmt.Sprintf("%s-trusted-%d", spoofMigrationID, time.Now().UnixNano())
			seedClusterName := fmt.Sprintf("e2e-migration-topic-seed-%d", time.Now().UnixNano())
			seedMSAName := fmt.Sprintf("e2e-migration-topic-msa-%d", time.Now().UnixNano())

			seedInFlightMigrationState(
				publisher,
				sourceHubName,
				targetHubName,
				migrationID,
				seedMSAName,
				seedClusterName,
			)

			// Wait for validating to settle and align agent migration state (transport-identity
			// suite may leave a stale processingMigrationId on the target hub agent).
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

			evt := migrationDeployingEvent(sourceHubName, targetHubName, trustedMigration, migrationID)
			Expect(publisher.SendToTopic(ctx, publisher.MigrationTopic(), evt)).To(Succeed(),
				"expected trusted migration deploying event to publish on gh-migration")

			Eventually(func() error {
				ns := &corev1.Namespace{}
				return targetHubClient.Get(ctx, types.NamespacedName{Name: trustedMigration}, ns)
			}, 2*time.Minute, 2*time.Second).Should(Succeed(),
				"trusted migration deploying event on gh-migration must create target resources")
		})

		It("should drop migration deploying events on gh-migration from an untrusted source hub", func() {
			migrationID := fmt.Sprintf("%s-untrusted-%d", spoofMigrationID, time.Now().UnixNano())
			seedClusterName := fmt.Sprintf("e2e-migration-topic-spoof-seed-%d", time.Now().UnixNano())
			seedMSAName := fmt.Sprintf("e2e-migration-topic-spoof-msa-%d", time.Now().UnixNano())

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

			evt := migrationDeployingEvent(spoofMigrationSource, targetHubName, spoofMigrationNS, migrationID)
			Expect(publisher.SendToTopic(ctx, publisher.MigrationTopic(), evt)).To(Succeed(),
				"expected spoofed migration event to publish on gh-migration for rejection testing")

			Consistently(func() error {
				ns := &corev1.Namespace{}
				err := targetHubClient.Get(ctx, types.NamespacedName{Name: spoofMigrationNS}, ns)
				if err == nil {
					return fmt.Errorf("namespace %q must not be created from spoofed migration source on %s",
						spoofMigrationNS, migrationTopic)
				}
				if client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			}, 45*time.Second, 500*time.Millisecond).Should(Succeed(),
				"spoofed migration deploying event on gh-migration must not create resources")
		})
	})
})
