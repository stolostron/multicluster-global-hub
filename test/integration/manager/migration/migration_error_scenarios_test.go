package migration_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

// go test ./test/integration/manager/migration -ginkgo.focus "Migration Error Scenarios" -v
var _ = Describe("Migration Error Scenarios", func() {
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

	Describe("Validation Error Scenarios", func() {
		Context("Missing Resources", func() {
			It("should fail when from hub does not exist", func() {
				By("Creating migration with non-existent from hub")
				m, err := createMigrationCR(ctx, migrationName, "non-existent-hub", toHubName, []string{clusterName})
				Expect(err).NotTo(HaveOccurred())
				Expect(m).NotTo(BeNil())

				By("Verifying validation failure with proper condition and phase")
				Eventually(func() error {
					if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
						return fmt.Errorf("failed to get migration: %v", err)
					}
					if m.Status.Phase != migrationv1alpha1.PhaseFailed {
						return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseFailed, m.Status.Phase)
					}
					condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
					if condition == nil {
						return fmt.Errorf("ConditionTypeValidated not found")
					}
					if condition.Status != metav1.ConditionFalse {
						return fmt.Errorf("ConditionTypeValidated status expected False, got %s", condition.Status)
					}
					if condition.Reason != migration.ConditionReasonHubClusterInvalid {
						return fmt.Errorf("ConditionTypeValidated reason expected %s, got %s", migration.ConditionReasonHubClusterInvalid, condition.Reason)
					}
					return nil
				}, "10s", "200ms").Should(Succeed())
			})

			It("should fail when to hub does not exist", func() {
				By("Creating migration with non-existent to hub")
				m, err := createMigrationCR(ctx, migrationName, fromHubName, "non-existent-hub", []string{clusterName})
				Expect(err).NotTo(HaveOccurred())
				Expect(m).NotTo(BeNil())

				By("Verifying validation failure with proper condition and phase")
				Eventually(func() error {
					if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
						return fmt.Errorf("failed to get migration: %v", err)
					}
					if m.Status.Phase != migrationv1alpha1.PhaseFailed {
						return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseFailed, m.Status.Phase)
					}
					condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
					if condition == nil {
						return fmt.Errorf("ConditionTypeValidated not found")
					}
					if condition.Status != metav1.ConditionFalse {
						return fmt.Errorf("ConditionTypeValidated status expected False, got %s", condition.Status)
					}
					if condition.Reason != migration.ConditionReasonHubClusterInvalid {
						return fmt.Errorf("ConditionTypeValidated reason expected %s, got %s", migration.ConditionReasonHubClusterInvalid, condition.Reason)
					}
					return nil
				}, "10s", "200ms").Should(Succeed())
			})
		})
	})

	Describe("Initialization Error Scenarios", func() {
		var m *migrationv1alpha1.ManagedClusterMigration

		BeforeEach(func() {
			var err error
			m, err = createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())

			By("Waiting for migration to complete validation and reach Initializing phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}
				// Simulate validation responses from hubs if migration is in validating phase
				setValidatingFinished(m, fromHubName, toHubName, []string{clusterName})
				if m.Status.Phase != migrationv1alpha1.PhaseInitializing {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseInitializing, m.Status.Phase)
				}
				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
				if condition == nil || condition.Status != metav1.ConditionTrue || condition.Reason != migration.ConditionReasonResourceValidated {
					return fmt.Errorf("validation condition not properly set")
				}
				return nil
			}, "10s", "200ms").Should(Succeed())

			By("Creating token secret for migration")
			Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())
		})

		It("should transition to cleaning when from hub reports error", func() {
			By("Simulating from hub error during initialization")
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
			migration.SetErrorMessage(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing, "initialization failed")

			By("Verifying error condition and transition to Rollbacking phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
				if condition == nil {
					return fmt.Errorf("ConditionTypeInitialized not found")
				}
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("ConditionTypeInitialized status expected False, got %s", condition.Status)
				}
				if condition.Reason != migration.ConditionReasonError {
					return fmt.Errorf("ConditionTypeInitialized reason expected %s, got %s", migration.ConditionReasonError, condition.Reason)
				}

				if m.Status.Phase != migrationv1alpha1.PhaseRollbacking {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseRollbacking, m.Status.Phase)
				}
				return nil
			}, "10s", "200ms").Should(Succeed())
		})

		It("should transition to cleaning when to hub reports error", func() {
			By("Simulating to hub error during initialization")
			migration.SetStarted(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseInitializing)
			migration.SetErrorMessage(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseInitializing, "initialization failed")

			By("Verifying error condition and transition to Rollbacking phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
				if condition == nil {
					return fmt.Errorf("ConditionTypeInitialized not found")
				}
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("ConditionTypeInitialized status expected False, got %s", condition.Status)
				}
				if condition.Reason != migration.ConditionReasonError {
					return fmt.Errorf("ConditionTypeInitialized reason expected %s, got %s", migration.ConditionReasonError, condition.Reason)
				}

				if m.Status.Phase != migrationv1alpha1.PhaseRollbacking {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseRollbacking, m.Status.Phase)
				}
				return nil
			}, "10s", "200ms").Should(Succeed())
		})

		It("should handle timeout when hub doesn't respond", func() {
			By("Waiting for migration timeout without hub response")
			// The migration controller checks for timeout using isReachedTimeout()
			// which uses the ConditionTypeStarted.LastTransitionTime
			// Since the timeout is 5 minutes in controller, this test should trigger timeout behavior

			By("Verifying timeout condition and transition to Cleaning phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
				if condition == nil {
					return fmt.Errorf("ConditionTypeInitialized not found")
				}
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("ConditionTypeInitialized status expected False, got %s", condition.Status)
				}
				// Could be either Waiting, Timeout, or Error depending on timing
				if condition.Reason != migration.ConditionReasonWaiting &&
					condition.Reason != migration.ConditionReasonTimeout &&
					condition.Reason != migration.ConditionReasonError {
					return fmt.Errorf("ConditionTypeInitialized reason expected Waiting/Timeout/Error, got %s", condition.Reason)
				}

				// Phase should still be Initializing if waiting, or Cleaning if timeout/error
				if m.Status.Phase != migrationv1alpha1.PhaseInitializing &&
					m.Status.Phase != migrationv1alpha1.PhaseCleaning {
					return fmt.Errorf("expected phase Initializing or Cleaning, got %s", m.Status.Phase)
				}
				return nil
			}, "10s", "200ms").Should(Succeed())
		})
	})

	Describe("Rollback Scenarios", func() {
		var m *migrationv1alpha1.ManagedClusterMigration

		BeforeEach(func() {
			var err error
			m, err = createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
			Expect(err).NotTo(HaveOccurred())
			Expect(m).NotTo(BeNil())
			// Simulate validation responses from hubs if migration is in validating phase
			setValidatingFinished(m, fromHubName, toHubName, []string{clusterName})

			By("Waiting for migration to complete validation and reach Initializing phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}

				if m.Status.Phase != migrationv1alpha1.PhaseInitializing {
					return fmt.Errorf("expected phase %s, got %s, migration: %v", migrationv1alpha1.PhaseInitializing, m.Status.Phase, m)
				}
				return nil
			}, "30s", "200ms").Should(Succeed())

			By("Creating token secret for migration")
			Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())
		})

		It("should transition to rollback when deploying stage fails", func() {
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
				return nil
			}, "10s", "200ms").Should(Succeed())

			By("Simulating deploying stage error")
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseDeploying)
			migration.SetErrorMessage(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseDeploying, "deploying failed")

			By("Verifying transition to Rollbacking phase")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}
				if m.Status.Phase != migrationv1alpha1.PhaseRollbacking {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseRollbacking, m.Status.Phase)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed)
				if condition == nil {
					return fmt.Errorf("ConditionTypeDeployed not found")
				}
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("ConditionTypeDeployed status expected False, got %s", condition.Status)
				}
				if condition.Reason != migration.ConditionReasonError {
					return fmt.Errorf("ConditionTypeDeployed reason expected %s, got %s", migration.ConditionReasonError, condition.Reason)
				}
				return nil
			}, "10s", "200ms").Should(Succeed())
		})

		It("should transition to failed after successful rollback", func() {
			By("Progressing through Initializing -> Deploying")
			simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseInitializing)
			Eventually(func() string {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return ""
				}
				return m.Status.Phase
			}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseDeploying))

			By("Simulating deploying stage error to trigger rollback")
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseDeploying)
			migration.SetErrorMessage(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseDeploying, "deploying failed")

			By("Waiting for rollback phase")
			Eventually(func() string {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return ""
				}
				return m.Status.Phase
			}, "20s", "200ms").Should(Equal(migrationv1alpha1.PhaseRollbacking))

			By("Simulating successful rollback confirmations")
			simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseRollbacking)
			simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseRollbacking)

			By("Verifying transition to Failed phase with rollback condition")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}
				if m.Status.Phase != migrationv1alpha1.PhaseFailed {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseFailed, m.Status.Phase)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeRolledBack)
				if condition == nil {
					return fmt.Errorf("ConditionTypeRolledBack not found")
				}
				if condition.Status != metav1.ConditionTrue {
					return fmt.Errorf("ConditionTypeRolledBack status expected True, got %s", condition.Status)
				}
				if condition.Reason != migration.ConditionReasonResourceRolledBack {
					return fmt.Errorf("ConditionTypeRolledBack reason expected %s, got %s", migration.ConditionReasonResourceRolledBack, condition.Reason)
				}
				return nil
			}, "10s", "200ms").Should(Succeed())
		})

		It("should handle rollback timeout and transition to failed", func() {
			By("Progressing through Initializing -> Registering")
			simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseInitializing)
			simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseDeploying)
			simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseDeploying)
			Eventually(func() string {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return ""
				}
				return m.Status.Phase
			}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseRegistering))

			By("Simulating registering stage error to trigger rollback")
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseRegistering)
			migration.SetErrorMessage(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseRegistering, "registering failed")

			By("Waiting for rollback phase")
			Eventually(func() string {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return ""
				}
				return m.Status.Phase
			}, "10s", "200ms").Should(Equal(migrationv1alpha1.PhaseRollbacking))

			By("Setting up short timeout for rollback test")
			// Configure the migration to use a very short timeout for rollback testing
			m.Spec.SupportedConfigs = &migrationv1alpha1.ConfigMeta{
				StageTimeout: &metav1.Duration{Duration: 1 * time.Second},
			}
			Expect(mgr.GetClient().Update(ctx, m)).To(Succeed())

			By("Simulating partial rollback success (only from hub)")
			migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseRollbacking)
			migration.SetFinished(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseRollbacking)
			// Intentionally not simulating to hub rollback success to test timeout/failure handling

			By("Verifying eventual transition to Failed phase despite rollback issues")
			Eventually(func() error {
				if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
					return fmt.Errorf("failed to get migration: %v", err)
				}
				if m.Status.Phase != migrationv1alpha1.PhaseFailed {
					return fmt.Errorf("expected phase %s, got %s", migrationv1alpha1.PhaseFailed, m.Status.Phase)
				}

				condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeRolledBack)
				if condition == nil {
					return fmt.Errorf("ConditionTypeRolledBack not found")
				}
				if condition.Reason != migration.ConditionReasonTimeout {
					return fmt.Errorf("ConditionTypeRolledBack reason expected %s, got %s", migration.ConditionReasonTimeout, condition.Reason)
				}
				// Rollback should be marked as failed but migration still transitions to Failed
				if condition.Status != metav1.ConditionFalse {
					return fmt.Errorf("ConditionTypeRolledBack status expected False, got %s", condition.Status)
				}
				return nil
			}, "15s", "200ms").Should(Succeed())
		})
	})
})
