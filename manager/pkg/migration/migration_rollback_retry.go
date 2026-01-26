package migration

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// handleRollbackRetryRequest checks if a failed migration has the rollback retry annotation
// and resets it to PhaseRollbacking if so. This allows users to manually trigger a rollback
// retry for migrations that failed during the rollback phase.
func (m *ClusterMigrationController) handleRollbackRetryRequest(ctx context.Context,
	req ctrl.Request,
) (bool, error) {
	mcm := &migrationv1alpha1.ManagedClusterMigration{}
	if err := m.Get(ctx, req.NamespacedName, mcm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Only process failed migrations with the rollback retry annotation
	if mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
		return false, nil
	}

	annotations := mcm.GetAnnotations()
	if annotations == nil {
		return false, nil
	}

	requestValue, exists := annotations[constants.MigrationRequestAnnotationKey]
	if !exists || requestValue != constants.MigrationRollbackAnnotationValue {
		return false, nil
	}
	// Ensure migration status exists in memory cache before operations
	AddMigrationStatus(string(mcm.GetUID()))

	log.Infof("rollback retry requested for migration %s (uid: %s)", mcm.Name, mcm.UID)

	// Remove annotation and update status in a single retry loop
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		// Remove the annotation to prevent infinite retry loops
		annotations := mcm.GetAnnotations()
		delete(annotations, constants.MigrationRequestAnnotationKey)
		mcm.SetAnnotations(annotations)
		if err := m.Update(ctx, mcm); err != nil {
			return err
		}
		// Remove the existing RolledBack and Cleaned conditions to reset LastTransitionTime
		meta.RemoveStatusCondition(&mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRolledBack)
		// Add new condition with fresh LastTransitionTime
		meta.SetStatusCondition(&mcm.Status.Conditions, metav1.Condition{
			Type:               migrationv1alpha1.ConditionTypeRolledBack,
			Status:             metav1.ConditionFalse,
			Reason:             ConditionReasonWaiting,
			Message:            "Rollback retry requested via annotation",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		mcm.Status.Phase = migrationv1alpha1.PhaseRollbacking
		return m.Status().Update(ctx, mcm)
	}); err != nil {
		return false, fmt.Errorf("failed to reset migration to rollbacking phase: %w", err)
	}

	// Reset the rollback stage states in memory cache to allow re-execution
	ResetStageState(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseRollbacking)
	ResetStageState(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking)

	log.Infof("migration %s reset to rollbacking phase for retry", mcm.Name)
	return true, nil
}
