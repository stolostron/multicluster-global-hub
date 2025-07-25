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
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
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
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{"non-existent-cluster"})
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
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
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

		By("Simulating hub confirmations") // can set the status based on the received event
		migration.SetStarted(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
		migration.SetFinished(string(m.GetUID()), fromHubName, migrationv1alpha1.PhaseInitializing)
		migration.SetStarted(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseInitializing)
		migration.SetFinished(string(m.GetUID()), toHubName, migrationv1alpha1.PhaseInitializing)

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
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
		Expect(err).NotTo(HaveOccurred(), "Failed to create migration CR")
		Expect(m).NotTo(BeNil())

		By("Create managedclusteraddon and mock the token secret")
		Expect(ensureManagedServiceAccount(m.Name, toHubName)).To(Succeed())

		By("Simulating hub error")
		pahse := migrationv1alpha1.PhaseInitializing
		migration.SetStarted(string(m.GetUID()), fromHubName, pahse)
		migration.SetErrorMessage(string(m.GetUID()), fromHubName, pahse, "initialization failed")

		By("Verifying error condition")
		Eventually(func() bool {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return false
			}
			condition := meta.FindStatusCondition(m.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			return condition != nil && condition.Status == metav1.ConditionFalse && condition.Reason == migration.ConditionReasonError
		}, "10s", "200ms").Should(BeTrue())

		By("Verifying transition to Cleaning phase")
		Eventually(func() error {
			if err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m); err != nil {
				return fmt.Errorf("failed to get the migration instance: %v", err)
			}
			if m.Status.Phase == migrationv1alpha1.PhaseCleaning {
				return nil
			}
			return fmt.Errorf("should get the Cleaning stage, but got: %s", m.Status.Phase)
		}, "10s", "200ms").Should(Succeed())
	})

	It("should complete full successful migration lifecycle", func() {
		By("Creating migration CR")
		m, err := createMigrationCR(ctx, migrationName, fromHubName, toHubName, []string{clusterName})
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
			return nil
		}, "10s", "200ms").Should(Succeed())

		By("Completing Cleaning -> Completed")
		simulateHubConfirmation(m.GetUID(), fromHubName, migrationv1alpha1.PhaseCleaning)
		simulateHubConfirmation(m.GetUID(), toHubName, migrationv1alpha1.PhaseCleaning)
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
			return nil
		}, "10s", "200ms").Should(Succeed())
	})
})
