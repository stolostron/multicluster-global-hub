package migration_test

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

// assertMigrationPhase verifies that a migration reaches the expected phase
func assertMigrationPhase(ctx context.Context, m *migrationv1alpha1.ManagedClusterMigration, expectedPhase string) {
	Eventually(func(g Gomega) {
		key := types.NamespacedName{Name: m.Name, Namespace: m.Namespace}
		g.Expect(mgr.GetClient().Get(ctx, key, m)).To(Succeed())
		g.Expect(m.Status.Phase).To(Equal(expectedPhase), fmt.Sprintf("Expected phase %s but got %s", expectedPhase, m.Status.Phase))
	}, 15*time.Second, 500*time.Millisecond).Should(Succeed())
}

// simulateHubConfirmation simulates a hub completing a migration phase
func simulateHubConfirmation(uid types.UID, hubName, phase string) {
	migration.SetStarted(string(uid), hubName, phase)
	migration.SetFinished(string(uid), hubName, phase)
}

// simulateHubError simulates a hub encountering an error during a migration phase
func simulateHubError(uid types.UID, hubName, phase, errorMsg string) {
	migration.SetStarted(string(uid), hubName, phase)
	migration.SetErrorMessage(string(uid), hubName, phase, errorMsg)
}

// completeFullMigration completes all phases of a migration
func completeFullMigration(ctx context.Context, m *migrationv1alpha1.ManagedClusterMigration) {
	// Create token secret
	createMockTokenSecret(ctx, m.Name, m.Spec.To)

	// Simulate all hub confirmations for each phase
	phases := []string{
		migrationv1alpha1.PhaseInitializing,
		migrationv1alpha1.PhaseDeploying,
		migrationv1alpha1.PhaseRegistering,
		migrationv1alpha1.PhaseCleaning,
	}

	for _, phase := range phases {
		// Wait for the migration to reach this phase
		Eventually(func() string {
			Expect(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m)).To(Succeed())
			return m.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(phase))

		// Provide confirmations
		simulateHubConfirmation(m.GetUID(), m.Spec.From, phase)
		simulateHubConfirmation(m.GetUID(), m.Spec.To, phase)

		// Small delay to allow processing
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for completion
	Eventually(func() string {
		Expect(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m)).To(Succeed())
		return m.Status.Phase
	}, 10*time.Second, 500*time.Millisecond).Should(Equal(migrationv1alpha1.PhaseCompleted))
}

// progressToPhase progresses a migration to a specific phase
func progressToPhase(ctx context.Context, m *migrationv1alpha1.ManagedClusterMigration, targetPhase string) {
	// Progress through phases until we reach the target phase
	phases := []string{
		migrationv1alpha1.PhaseValidating,
		migrationv1alpha1.PhaseInitializing,
		migrationv1alpha1.PhaseDeploying,
		migrationv1alpha1.PhaseRegistering,
		migrationv1alpha1.PhaseCleaning,
	}

	for _, phase := range phases {
		Eventually(func() string {
			Expect(mgr.GetClient().Get(ctx, client.ObjectKeyFromObject(m), m)).To(Succeed())
			return m.Status.Phase
		}, 10*time.Second, 500*time.Millisecond).Should(Equal(phase))

		if phase == targetPhase {
			break
		}

		// Provide necessary confirmations to progress
		switch phase {
		case migrationv1alpha1.PhaseValidating:
			// Wait for automatic validation
			continue
		case migrationv1alpha1.PhaseInitializing:
			createMockTokenSecret(ctx, m.Name, m.Spec.To)
			simulateHubConfirmation(m.GetUID(), m.Spec.From, phase)
			simulateHubConfirmation(m.GetUID(), m.Spec.To, phase)
		case migrationv1alpha1.PhaseDeploying:
			simulateHubConfirmation(m.GetUID(), m.Spec.From, phase)
			simulateHubConfirmation(m.GetUID(), m.Spec.To, phase)
		case migrationv1alpha1.PhaseRegistering:
			simulateHubConfirmation(m.GetUID(), m.Spec.From, phase)
			simulateHubConfirmation(m.GetUID(), m.Spec.To, phase)
		case migrationv1alpha1.PhaseCleaning:
			simulateHubConfirmation(m.GetUID(), m.Spec.From, phase)
			simulateHubConfirmation(m.GetUID(), m.Spec.To, phase)
		}
	}
}

// assertMigrationCondition verifies that a migration has the expected condition
func assertMigrationConditionHelper(ctx context.Context, m *migrationv1alpha1.ManagedClusterMigration,
	conditionType string, status metav1.ConditionStatus, reason string,
) {
	Eventually(func(g Gomega) {
		key := types.NamespacedName{Name: m.Name, Namespace: m.Namespace}
		g.Expect(mgr.GetClient().Get(ctx, key, m)).To(Succeed())

		cond := meta.FindStatusCondition(m.Status.Conditions, conditionType)
		g.Expect(cond).NotTo(BeNil(), fmt.Sprintf("Condition %s not found", conditionType))
		g.Expect(cond.Status).To(Equal(status), fmt.Sprintf("Expected status %s but got %s", status, cond.Status))
		g.Expect(cond.Reason).To(Equal(reason), fmt.Sprintf("Expected reason %s but got %s", reason, cond.Reason))
	}, 10*time.Second, 100*time.Millisecond).Should(Succeed())
}

// containsTestID checks if a name contains the test ID
func containsTestID(name, testID string) bool {
	return len(name) > len(testID) && name[len(name)-len(testID):] == testID
}
