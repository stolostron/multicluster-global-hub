package migration

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	ConditionReasonStarted = "InstanceStarted"
)

// getCurrentMigration returns the current migration object.
// It returns nil if no migration is in progress.
// It selects the oldest migration object if there are multiple migrations.
// For other migrations which are waiting for migrating, it will update the condition and phase to pending.
func (m *ClusterMigrationController) getCurrentMigration(ctx context.Context,
	req ctrl.Request,
) (*migrationv1alpha1.ManagedClusterMigration, error) {
	migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
	err := m.List(ctx, migrationList)
	if err != nil {
		return nil, err
	}

	condType := migrationv1alpha1.ConditionTypeStarted
	condStatus := metav1.ConditionFalse
	condReason := ConditionReasonWaiting
	condMessage := "Waiting for the migration to be started"

	// update the migration condition to false if the phase is empty
	for i := range migrationList.Items {
		migration := &migrationList.Items[i]

		if migration.Status.Phase == "" {
			err := m.UpdateCondition(ctx, migration, condType, condStatus, condReason, condMessage)
			if err != nil {
				log.Errorf("failed to update migration condition: %v", err)
				return nil, err
			}
		}

		// return the migration if it is the deletion request
		if !migration.GetDeletionTimestamp().IsZero() && req.Name == migration.Name {
			return migration, nil
		}
	}

	var nextMigration *migrationv1alpha1.ManagedClusterMigration

	for i := range migrationList.Items {
		migration := &migrationList.Items[i]

		// Skip final states (completed or failed) - these are truly done
		if migration.Status.Phase == migrationv1alpha1.PhaseCompleted ||
			migration.Status.Phase == migrationv1alpha1.PhaseFailed {
			continue
		}

		if nextMigration == nil {
			nextMigration = migration
		} else if migration.CreationTimestamp.Before(&nextMigration.CreationTimestamp) {
			nextMigration = migration
		}
	}

	if nextMigration != nil {
		condStatus = metav1.ConditionTrue
		condReason = ConditionReasonStarted
		condMessage = "Migration instance is started"
		if err := m.UpdateCondition(ctx, nextMigration, condType, condStatus, condReason, condMessage); err != nil {
			log.Errorf("failed to update migration condition: %v", err)
		}
	}

	return nextMigration, nil
}
