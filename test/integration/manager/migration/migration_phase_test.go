package migration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ = Describe("Migration Phase Transitions - Simplified", func() {
	var ctx context.Context
	var cancel context.CancelFunc
	var testID, fromHubName, toHubName, clusterName, migrationName string

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())
		testID = fmt.Sprintf("test-%d", time.Now().UnixNano())
		fromHubName = "hub1-" + testID
		toHubName = "hub2-" + testID
		clusterName = "cluster-" + testID
		migrationName = "migration-" + testID

		specEvents = []*cloudevents.Event{}

		Expect(createHubAndCluster(ctx, fromHubName, clusterName)).To(Succeed())
		Expect(createHubAndCluster(ctx, toHubName, "temp-cluster-for-hub-creation")).To(Succeed())
	})

	AfterEach(func() {
		By("Cleaning up test resources")
		cleanupHubAndClusters(ctx, fromHubName, clusterName)
		cleanupHubAndClusters(ctx, toHubName, "temp-cluster-for-hub-creation")

		By("Cleaning up migration CR and waiting for deletion")
		Expect(cleanupMigrationCR(ctx, migrationName, "default")).To(Succeed())

		By("Ensuring no migrations are running before next test")
		Eventually(func() bool {
			migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
			if err := mgr.GetClient().List(ctx, migrationList); err != nil {
				return false
			}
			for _, m := range migrationList.Items {
				if m.Status.Phase != migrationv1alpha1.PhaseCompleted &&
					m.Status.Phase != migrationv1alpha1.PhaseFailed &&
					m.DeletionTimestamp == nil {
					return false
				}
			}
			return true
		}, "10s", "200ms").Should(BeTrue())

		cancel()
	})

	It("should transition from creation to Initializing phase", func() {
		By("Creating migration CR")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName}, []string{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		By("Verifying validation condition was set correctly")
		Eventually(func() bool {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return false
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			return condition != nil && condition.Status == metav1.ConditionTrue && condition.Reason == migration.ConditionReasonResourceValidated
		}, "5s", "200ms").Should(BeTrue())

		By("Verifying migration reaches Initializing phase")
		Eventually(func() string {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return ""
			}
			return m.Status.Phase
		}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseInitializing))
	})

	It("should fail validation for non-existent cluster", func() {
		By("Creating migration with non-existent cluster")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{"non-existent-cluster"}, []string{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		By("Verifying validation failure")
		Eventually(func() bool {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return false
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == migration.ConditionReasonClusterNotFound
		}, "10s", "200ms").Should(BeTrue())

		By("Verifying transition to Failed phase without cleanup stage")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get the migration instance %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseFailed {
				return fmt.Errorf("the migration phasing should change into Failed, but got: %s", m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned)
			if condition != nil {
				return fmt.Errorf("the clean up condition should not be set in the validated, condition: %v", condition)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())
	})

	It("should progress from Initializing to Deploying with proper setup", func() {
		By("Creating migration CR and reaching Initializing")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName}, []string{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		Eventually(func() string {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return ""
			}
			return m.Status.Phase
		}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseInitializing))

		By("Create managedclusteraddon and mock the token secret")
		Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())

		By("Verifying initial waiting state")
		Eventually(func() bool {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return false
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == migration.ConditionReasonWaiting
		}, "10s", "200ms").Should(BeTrue())

		By("Simulating target hub confirmation by event")
		Eventually(func() error {
			expectedTargetEvent := cloudevents.NewEvent()
			expectedTargetEvent.SetType(constants.MigrationTargetMsgKey)
			expectedTargetEvent.SetSource(constants.CloudEventGlobalHubClusterName)
			expectedTargetEvent.SetExtension(constants.CloudEventExtensionKeyClusterName, toHubName)

			data, err := json.Marshal(&migrationbundle.MigrationTargetHubBundle{
				MigrationId:                           string(m.GetUID()),
				Stage:                                 migrationv1alpha1.PhaseInitializing,
				ManagedServiceAccountName:             migrationName,
				ManagedServiceAccountInstallNamespace: "open-cluster-management-agent-addon",
				// ManagedClusters:                       []string{clusterName},
				// Rollback:                              false,
			})
			if err != nil {
				return fmt.Errorf("failed to marshal target bundle: %v", err)
			}
			expectedTargetEvent.SetData(cloudevents.ApplicationJSON, data)

			if err := validateSpecEvent(specEvents, expectedTargetEvent); err != nil {
				return fmt.Errorf("failed to validate spec event: %v", err)
			}

			// set the finished event to the migration
			migration.SetFinished(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseInitializing)
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Simulating source hub confirmation by event")
		Eventually(func() error {
			expectedSourceEvent := cloudevents.NewEvent()
			expectedSourceEvent.SetType(constants.MigrationSourceMsgKey)
			expectedSourceEvent.SetSource(constants.CloudEventGlobalHubClusterName)
			expectedSourceEvent.SetExtension(constants.CloudEventExtensionKeyClusterName, fromHubName)

			data, err := json.Marshal(&migrationbundle.MigrationSourceHubBundle{
				MigrationId:     string(m.GetUID()),
				Stage:           migrationv1alpha1.PhaseInitializing,
				ToHub:           toHubName,
				ManagedClusters: []string{clusterName},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bootstrap-" + toHubName,
						Namespace: "multicluster-engine",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("kubeconfig"),
					},
				},
			})
			if err != nil {
				return fmt.Errorf("failed to marshal source bundle: %v", err)
			}
			expectedSourceEvent.SetData(cloudevents.ApplicationJSON, data)

			if err := validateSpecEvent(specEvents, expectedSourceEvent); err != nil {
				return fmt.Errorf("failed to validate spec event: %v", err)
			}

			migration.SetFinished(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Verifying initialization completion")
		Eventually(func() bool {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return false
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			return condition != nil && condition.Status == metav1.ConditionTrue && condition.Reason == migration.ConditionReasonResourceInitialized
		}, "20s", "200ms").Should(BeTrue())

		By("Transitioning to Deploying phase")
		Eventually(func() string {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return ""
			}
			return m.Status.Phase
		}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseDeploying))
	})

	It("should handle hub error during initialization", func() {
		By("Creating migration and reaching Initializing")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName}, []string{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		By("Create managedclusteraddon and mock the token secret")
		Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())

		By("Simulating hub error")
		phase := migrationv1alpha1.PhaseInitializing
		migration.SetStarted(string(m.GetUID()), fromHubName, phase)
		migration.SetErrorMessage(string(m.GetUID()), fromHubName, phase, "initialization failed")

		By("Verifying error condition and transition to Cleaning phase")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return err
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			if condition == nil {
				return fmt.Errorf("condition not found")
			}
			if condition.Status != metav1.ConditionFalse {
				return fmt.Errorf("condition status expected False, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonError {
				return fmt.Errorf("condition reason expected %s, got %s", migration.ConditionReasonError, condition.Reason)
			}

			if m.Status.Phase != migrationv1alpha1.PhaseCleaning {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseCleaning, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Simulating target hub confirmation by event")
		Eventually(func() error {
			expectedTargetEvent := cloudevents.NewEvent()
			expectedTargetEvent.SetType(constants.MigrationTargetMsgKey)
			expectedTargetEvent.SetSource(constants.CloudEventGlobalHubClusterName)
			expectedTargetEvent.SetExtension(constants.CloudEventExtensionKeyClusterName, toHubName)

			data, err := json.Marshal(&migrationbundle.MigrationTargetHubBundle{
				MigrationId:                           string(m.GetUID()),
				Stage:                                 migrationv1alpha1.PhaseCleaning,
				ManagedServiceAccountName:             migrationName,
				ManagedServiceAccountInstallNamespace: "open-cluster-management-agent-addon",
				ManagedClusters:                       []string{clusterName},
				Rollback:                              true,
			})
			if err != nil {
				return fmt.Errorf("failed to marshal target bundle: %v", err)
			}
			expectedTargetEvent.SetData(cloudevents.ApplicationJSON, data)

			if err := validateSpecEvent(specEvents, expectedTargetEvent); err != nil {
				return fmt.Errorf("failed to validate spec event: %v", err)
			}

			// set the finished event to the migration
			migration.SetFinished(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseCleaning)
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Simulating source hub confirmation by event")
		Eventually(func() error {
			expectedSourceEvent := cloudevents.NewEvent()
			expectedSourceEvent.SetType(constants.MigrationSourceMsgKey)
			expectedSourceEvent.SetSource(constants.CloudEventGlobalHubClusterName)
			expectedSourceEvent.SetExtension(constants.CloudEventExtensionKeyClusterName, fromHubName)

			data, err := json.Marshal(&migrationbundle.MigrationSourceHubBundle{
				MigrationId:     string(m.GetUID()),
				Stage:           migrationv1alpha1.PhaseCleaning,
				ToHub:           toHubName,
				ManagedClusters: []string{clusterName},
				BootstrapSecret: &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "bootstrap-" + toHubName,
						Namespace: "multicluster-engine",
					},
					Data: map[string][]byte{
						"kubeconfig": []byte("kubeconfig"),
					},
				},
				Rollback: true,
			})
			if err != nil {
				return fmt.Errorf("failed to marshal source bundle: %v", err)
			}
			expectedSourceEvent.SetData(cloudevents.ApplicationJSON, data)

			if err := validateSpecEvent(specEvents, expectedSourceEvent); err != nil {
				return fmt.Errorf("failed to validate spec event: %v", err)
			}

			migration.SetFinished(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseCleaning)
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Verifying transition to Failed phase")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get the migration instance: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseFailed {
				return fmt.Errorf("should get the Failed stage, but got: %s", m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())
	})

	It("should complete full successful migration lifecycle", func() {
		By("Creating migration CR")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName}, []string{})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		By("Verifying validation and reaching Initializing phase")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get migration: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseInitializing {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseInitializing, m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			if condition == nil {
				return fmt.Errorf("ConditionTypeValidated not found")
			}
			if condition.Status != metav1.ConditionTrue {
				return fmt.Errorf("ConditionTypeValidated status expected True, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonResourceValidated {
				return fmt.Errorf("ConditionTypeValidated reason expected %s, got %s", migration.ConditionReasonResourceValidated, condition.Reason)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseInitializing {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseInitializing, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Creating token secret")
		Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())

		By("Progressing through Initializing -> Deploying")
		simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseInitializing)
		simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseInitializing)
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get migration: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseDeploying {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseDeploying, m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			if condition == nil {
				return fmt.Errorf("ConditionTypeInitialized not found")
			}
			if condition.Status != metav1.ConditionTrue {
				return fmt.Errorf("ConditionTypeInitialized status expected True, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonResourceInitialized {
				return fmt.Errorf("ConditionTypeInitialized reason expected %s, got %s", migration.ConditionReasonResourceInitialized, condition.Reason)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseDeploying {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseDeploying, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Progressing through Deploying -> Registering")
		simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseDeploying)
		simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseDeploying)
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get migration: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseRegistering {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseRegistering, m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed)
			if condition == nil {
				return fmt.Errorf("ConditionTypeDeployed not found")
			}
			if condition.Status != metav1.ConditionTrue {
				return fmt.Errorf("ConditionTypeDeployed status expected True, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonResourcesDeployed {
				return fmt.Errorf("ConditionTypeDeployed reason expected %s, got %s", migration.ConditionReasonResourcesDeployed, condition.Reason)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseRegistering {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseRegistering, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Progressing through Registering -> Cleaning")
		simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseRegistering)
		simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseRegistering)
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get migration: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseCleaning {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseCleaning, m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered)
			if condition == nil {
				return fmt.Errorf("ConditionTypeRegistered not found")
			}
			if condition.Status != metav1.ConditionTrue {
				return fmt.Errorf("ConditionTypeRegistered status expected True, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonClusterRegistered {
				return fmt.Errorf("ConditionTypeRegistered reason expected %s, got %s", migration.ConditionReasonClusterRegistered, condition.Reason)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseCleaning {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseCleaning, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Verifying cleaning events and marking the cleaning as finished")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get the migration instance: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseCleaning {
				return fmt.Errorf("should get the Cleaning stage, but got: %s", m.Status.Phase)
			}

			// verify sending the rollback cleaning event to both source and target hub
			confirmations := 0
			for _, event := range specEvents {

				if event.Source() != constants.CloudEventGlobalHubClusterName {
					continue
				}

				// target hub cleaning
				if event.Type() == constants.MigrationTargetMsgKey && event.Extensions()[constants.CloudEventExtensionKeyClusterName] == toHubName {
					fromHubBundle := &migrationbundle.MigrationTargetHubBundle{}
					if err := json.Unmarshal(event.Data(), fromHubBundle); err != nil {
						return fmt.Errorf("failed to unmarshal from hub event: %v", err)
					}
					fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>> received to hub event:")
					fmt.Println(event)

					if fromHubBundle.MigrationId != string(m.GetUID()) {
						return fmt.Errorf("migration id mismatch, expected %s, got %s", string(m.GetUID()), fromHubBundle.MigrationId)
					}

					if fromHubBundle.Stage != migrationv1alpha1.PhaseCleaning {
						continue
					}

					// rollback should be false
					if fromHubBundle.Rollback {
						return fmt.Errorf("rollback should be false")
					}

					// cleaning the rbac of the service account
					if fromHubBundle.ManagedServiceAccountName != migrationName {
						return fmt.Errorf("managed service account name mismatch, expected %s, got %s", migrationName, fromHubBundle.ManagedServiceAccountName)
					}

					// cleaning the managed cluster
					if fromHubBundle.ManagedClusters[0] != clusterName {
						return fmt.Errorf("managed cluster mismatch, expected %s, got %s", clusterName, fromHubBundle.ManagedClusters[0])
					}

					migration.SetFinished(string(m.GetUID()), toHubName, fromHubBundle.Stage)
					confirmations++
				}

				// source hub cleaning
				if event.Type() == constants.MigrationSourceMsgKey && event.Extensions()[constants.CloudEventExtensionKeyClusterName] == fromHubName {

					sourceHubBundle := &migrationbundle.MigrationSourceHubBundle{}
					if err := json.Unmarshal(event.Data(), sourceHubBundle); err != nil {
						return fmt.Errorf("failed to unmarshal from hub event: %v", err)
					}

					fmt.Println(">>>>>>>>>>>>>>>>>>>>>>>>> received from hub event:")
					fmt.Println(event)

					if sourceHubBundle.MigrationId != string(m.GetUID()) {
						return fmt.Errorf("migration id mismatch, expected %s, got %s", string(m.GetUID()), sourceHubBundle.MigrationId)
					}

					if sourceHubBundle.Stage != migrationv1alpha1.PhaseCleaning {
						continue
					}

					// rollback should be false
					if sourceHubBundle.Rollback {
						return fmt.Errorf("rollback should be false")
					}

					// use to clean up the klusterletconfig(name = klusterletConfigNamePrefix + migratingEvt.ToHub)
					if sourceHubBundle.ToHub != toHubName {
						return fmt.Errorf("to hub mismatch, expected %s, got %s", toHubName, sourceHubBundle.ToHub)
					}

					if sourceHubBundle.ManagedClusters[0] != clusterName {
						return fmt.Errorf("managed cluster mismatch, expected %s, got %s", clusterName, sourceHubBundle.ManagedClusters[0])
					}

					// should not be nil
					if sourceHubBundle.BootstrapSecret == nil {
						return fmt.Errorf("bootstrap secret should not be nil")
					}

					migration.SetFinished(string(m.GetUID()), fromHubName, sourceHubBundle.Stage)
					confirmations++
				}
			}

			if confirmations == 2 {
				return nil
			}
			return fmt.Errorf("expected 2 cleaning confirmations, got %d", confirmations)
		}, "10s", "200ms").Should(Succeed())

		By("Completing Cleaning -> Completed")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get migration: %v", err)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseCompleted {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseCompleted, m.Status.Phase)
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned)
			if condition == nil {
				return fmt.Errorf("ConditionTypeCleaned not found")
			}
			if condition.Status != metav1.ConditionTrue {
				return fmt.Errorf("ConditionTypeCleaned status expected True, got %s", condition.Status)
			}
			if condition.Reason != migration.ConditionReasonResourceCleaned {
				return fmt.Errorf("ConditionTypeCleaned reason expected %s, got %s", migration.ConditionReasonResourceCleaned, condition.Reason)
			}
			if m.Status.Phase != migrationv1alpha1.PhaseCompleted {
				return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseCompleted, m.Status.Phase)
			}
			return nil
		}, "10s", "200ms").Should(Succeed())
	})
})
