package migration

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var deleteInterval = 5 * time.Minute

// Completed:
// 1. Once the resource deployed in the destination hub, Clean up the resources in the source hub
// 2. 5 minutes after completion: delete the migration CR, delete the completed items in db
func (m *ClusterMigrationController) completed(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted ||
		!meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		return false, nil
	}

	log.Infof("completed the migration: %s", mcm.Name)

	// clean up the source hub resource -> confirmationEvent: database items change into MigrationCompleted
	// BootstrapSecret, klusterletConfig, detaching clusters
	var deployed []models.ManagedClusterMigration
	err := database.GetGorm().Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationResourceDeployed,
	}).Find(&deployed).Error
	if err != nil {
		return false, err
	}

	cleaningClusters := map[string][]string{}
	for _, d := range deployed {
		cleaningClusters[d.FromHub] = append(cleaningClusters[d.FromHub], d.ClusterName)
	}
	bootstrapSecret := getBootstrapSecret(mcm.Spec.To, nil)
	for sourceHub, clusters := range cleaningClusters {
		log.Infof("cleaning up the source hub resources: %s", sourceHub)
		err = m.sendEventToSourceHub(ctx, sourceHub, mcm.Spec.To, migrationv1alpha1.MigrationCompleted,
			clusters, bootstrapSecret)
		if err != nil {
			log.Errorf("failed to send clean up event into source hub(%s)", sourceHub)
			return false, err
		}
	}
	if len(cleaningClusters) > 0 {
		log.Info("waiting resource to be cleaned up")
		return true, nil
	}

	// wait the completed up to 5 minutes, delete the CR, completed items in db
	cond := meta.FindStatusCondition(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed)
	interval := time.Since(cond.LastTransitionTime.Time)
	if interval < deleteInterval {
		log.Infof("migration completed, waiting to clean up: %f - %f", interval, deleteInterval.Seconds())
		return true, nil
	}

	log.Info("migration completed, cleaning up")
	err = m.Client.Delete(ctx, mcm)
	if err != nil && !errors.IsNotFound(err) {
		return true, err
	}

	err = database.GetGorm().Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationCompleted,
		ToHub: mcm.Spec.To,
	}).Delete(&models.ManagedClusterMigration{}).Error
	if err != nil {
		return false, err
	}

	return false, nil
}

func SetDeleteDuration(interval time.Duration) {
	deleteInterval = interval
}
