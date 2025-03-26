package migration

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

// Completed:
// 1. Olny handle when the migration phase is completed and the resource deployed
// 2. Delete the migration CR 5 minutes after completion
func (m *ClusterMigrationController) completed(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.Status.Phase != migrationv1alpha1.MigrationCompleted ||
		!meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		return false, nil
	}

	cond := meta.FindStatusCondition(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed)
	if time.Since(cond.LastTransitionTime.Time) < 5*time.Minute {
		log.Info("migration completed, waiting to clean up")
		return true, nil
	}

	log.Info("migration completed, cleaning up")

	err := m.Client.Delete(ctx, mcm)
	if err != nil && !errors.IsNotFound(err) {
		return true, err
	}
	return false, nil
}
