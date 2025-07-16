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

var _ = Describe("Migration Failure Handling", func() {
	It("should fail only on validation errors or non-waiting reasons", func() {
		// Test that validation errors trigger failure
		validationFailMigration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "validation-fail-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"nonexistent-cluster"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, validationFailMigration)).To(Succeed())

		// Should fail on validation error
		Eventually(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(validationFailMigration), validationFailMigration)
			if err != nil {
				return err
			}

			validationCond := meta.FindStatusCondition(validationFailMigration.Status.Conditions, migrationv1alpha1.ConditionTypeValidated)
			if validationCond == nil {
				return fmt.Errorf("validation condition not found")
			}

			// Should fail due to validation error (not waiting)
			if validationCond.Status == metav1.ConditionFalse && validationCond.Reason != migration.ConditionReasonWaiting {
				return nil
			}

			return fmt.Errorf("expected validation failure but got reason: %s", validationCond.Reason)
		}, 30*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(mgr.GetClient().Delete(ctx, validationFailMigration)).To(Succeed())
	})

	It("should not fail on waiting reasons", func() {
		// Test that waiting reasons don't trigger failure
		waitingOnlyMigration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "waiting-only-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
				IncludedManagedClusters: []string{"cluster1"},
				From:                    "hub1",
				To:                      "hub2",
			},
		}
		Expect(mgr.GetClient().Create(ctx, waitingOnlyMigration)).To(Succeed())

		// Should not fail due to waiting state
		Consistently(func() error {
			err := mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(waitingOnlyMigration), waitingOnlyMigration)
			if err != nil {
				return err
			}

			// Should not be in failed state due to waiting
			if waitingOnlyMigration.Status.Phase == migrationv1alpha1.PhaseFailed {
				return fmt.Errorf("migration should not fail due to waiting state")
			}
			return nil
		}, 10*time.Second, 100*time.Millisecond).Should(Succeed())

		// Cleanup
		Expect(mgr.GetClient().Delete(ctx, waitingOnlyMigration)).To(Succeed())
	})
})
