package migration

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
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

	// Check if this is a deletion request
	for i := range migrationList.Items {
		migration := &migrationList.Items[i]
		if !migration.GetDeletionTimestamp().IsZero() && req.Name == migration.Name {
			return migration, nil
		}
	}

	// Filter active migrations and select the desired one
	desiredMigration, pendingMigrations := m.selectActiveMigration(migrationList.Items)
	if desiredMigration == nil {
		return nil, nil
	}

	// Update conditions for desired and pending migrations
	if err := m.updateMigrationConditions(ctx, desiredMigration, pendingMigrations); err != nil {
		log.Errorf("failed to update migration conditions: %v", err)
	}

	return desiredMigration, nil
}

// selectActiveMigration selects the active migration from the list
func (m *ClusterMigrationController) selectActiveMigration(migrations []migrationv1alpha1.ManagedClusterMigration) (*migrationv1alpha1.ManagedClusterMigration, []migrationv1alpha1.ManagedClusterMigration) {
	var desiredMigration *migrationv1alpha1.ManagedClusterMigration
	var pendingMigrations []migrationv1alpha1.ManagedClusterMigration

	for i := range migrations {
		migration := &migrations[i]

		// Skip final states (completed or failed) - these are truly done
		if migration.Status.Phase == migrationv1alpha1.PhaseCompleted || migration.Status.Phase == migrationv1alpha1.PhaseFailed {
			continue
		}

		// Select the oldest migration as the active one
		if desiredMigration == nil {
			desiredMigration = migration
		} else if migration.CreationTimestamp.Before(&desiredMigration.CreationTimestamp) {
			pendingMigrations = append(pendingMigrations, *desiredMigration)
			desiredMigration = migration
		} else {
			pendingMigrations = append(pendingMigrations, *migration)
		}
	}

	return desiredMigration, pendingMigrations
}

// updateMigrationConditions updates the conditions for desired and pending migrations
func (m *ClusterMigrationController) updateMigrationConditions(ctx context.Context,
	desiredMigration *migrationv1alpha1.ManagedClusterMigration,
	pendingMigrations []migrationv1alpha1.ManagedClusterMigration) error {

	// Update desired migration with started condition
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Client.Get(ctx, client.ObjectKeyFromObject(desiredMigration), desiredMigration); err != nil {
			return err
		}

		if err := m.UpdateCondition(ctx,
			desiredMigration,
			migrationv1alpha1.ConditionTypeStarted,
			metav1.ConditionTrue,
			"migrationInProgress",
			"Migration is in progress",
		); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to update started condition for migration %s: %v", desiredMigration.Name, err)
	}

	// Update pending migrations
	for _, mcm := range pendingMigrations {
		e := m.UpdateConditionWithRetry(ctx,
			&mcm,
			migrationv1alpha1.ConditionTypeStarted,
			metav1.ConditionFalse,
			"waitOtherMigrationCompleted",
			fmt.Sprintf("Wait for other migration <%s> to be completed", desiredMigration.Name),
			migrationStageTimeout,
		)
		if e != nil {
			log.Errorf("failed to update pending condition for migration %s: %v", mcm.Name, e)
		}
	}

	return nil
}
