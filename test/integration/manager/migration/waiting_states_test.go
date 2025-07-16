package migration_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ = Describe("Migration Waiting States", func() {
	It("should use waiting state during initialization phase", func() {
		waitingMigration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "waiting-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"cluster1"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, waitingMigration)).To(Succeed())

		// Check that during initialization, the migration shows waiting state
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(waitingMigration), waitingMigration)
			if err != nil {
				return err
			}

			initCond := meta.FindStatusCondition(waitingMigration.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
			if initCond == nil {
				return fmt.Errorf("initialization condition not found")
			}

			if initCond.Status == metav1.ConditionFalse && initCond.Reason == migration.ConditionReasonWaiting {
				return nil
			}

			return fmt.Errorf("expected waiting state but got reason: %s", initCond.Reason)
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Migration should not be in failed state due to waiting
		Consistently(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(waitingMigration), waitingMigration)
			if err != nil {
				return err
			}

			if waitingMigration.Status.Phase == migrationv1alpha1.PhaseFailed {
				return fmt.Errorf("migration should not fail due to waiting state")
			}
			return nil
		}, 5*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(mgr.GetClient().Delete(ctx, waitingMigration)).To(Succeed())
	})

	It("should handle deployment waiting states correctly", func() {
		deployWaitingMigration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "deploy-waiting-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"cluster1"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, deployWaitingMigration)).To(Succeed())

		// Wait for migration to reach deployment phase
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(deployWaitingMigration), deployWaitingMigration)
			if err != nil {
				return err
			}

			if deployWaitingMigration.Status.Phase == migrationv1alpha1.PhaseDeploying {
				return nil
			}
			return fmt.Errorf("migration not in deploying phase yet")
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Check that deployment uses waiting state
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(deployWaitingMigration), deployWaitingMigration)
			if err != nil {
				return err
			}

			deployedCond := meta.FindStatusCondition(deployWaitingMigration.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed)
			if deployedCond == nil {
				return fmt.Errorf("deployment condition not found")
			}

			if deployedCond.Status == metav1.ConditionFalse && deployedCond.Reason == migration.ConditionReasonWaiting {
				return nil
			}

			return fmt.Errorf("expected waiting state during deployment but got reason: %s", deployedCond.Reason)
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(mgr.GetClient().Delete(ctx, deployWaitingMigration)).To(Succeed())
	})

	It("should handle registration waiting states correctly", func() {
		registerWaitingMigration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "register-waiting-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"cluster1"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, registerWaitingMigration)).To(Succeed())

		// Mock progression to registration phase
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(registerWaitingMigration), registerWaitingMigration)
			if err != nil {
				return err
			}

			// Mock phase completion
			migration.SetFinished(string(registerWaitingMigration.GetUID()), "hub1", migrationv1alpha1.PhaseInitializing)
			migration.SetFinished(string(registerWaitingMigration.GetUID()), "hub2", migrationv1alpha1.PhaseInitializing)
			migration.SetFinished(string(registerWaitingMigration.GetUID()), "hub1", migrationv1alpha1.PhaseDeploying)
			migration.SetFinished(string(registerWaitingMigration.GetUID()), "hub2", migrationv1alpha1.PhaseDeploying)

			if registerWaitingMigration.Status.Phase == migrationv1alpha1.PhaseRegistering {
				return nil
			}
			return fmt.Errorf("migration not in registering phase yet")
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Check that registration uses waiting state
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(registerWaitingMigration), registerWaitingMigration)
			if err != nil {
				return err
			}

			registerCond := meta.FindStatusCondition(registerWaitingMigration.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered)
			if registerCond == nil {
				return fmt.Errorf("registration condition not found")
			}

			if registerCond.Status == metav1.ConditionFalse && registerCond.Reason == migration.ConditionReasonWaiting {
				return nil
			}

			return fmt.Errorf("expected waiting state during registration but got reason: %s", registerCond.Reason)
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(mgr.GetClient().Delete(ctx, registerWaitingMigration)).To(Succeed())
	})
})
