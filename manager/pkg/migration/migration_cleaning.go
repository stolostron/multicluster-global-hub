package migration

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	conditionReasonResourceNotCleaned = "ResourceNotCleaned"
	conditionReasonResourceCleaned    = "ResourceCleaned"
)

var deleteInterval = 5 * time.Minute

// Completed:
// 1. Once the resource deployed in the destination hub, Clean up the resources in the source hub
func (m *ClusterMigrationController) completed(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseCleaning {
		return false, nil
	}

	log.Infof("cleaning the migration: %s", mcm.Name)

	condType := migrationv1alpha1.ConditionTypeCleaned
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonResourceCleaned
	condMsg := "Resources have been cleaned from the hub clusters"
	var err error

	defer func() {
		if err != nil {
			condMsg = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonResourceNotCleaned
		}
		log.Infof("cleaning condition %s(%s): %s", condType, condReason, condMsg)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMsg)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}
	}()

	// Deleting the ManagedServiceAccount will revoke the bootstrap kubeconfig secret of the migrated cluster.
	// Be cautious — this action may carry potential risks.
	if err := m.deleteManagedServiceAccount(ctx, mcm); err != nil {
		log.Errorf("failed to delete the managedServiceAccount: %s/%s", mcm.Spec.To, mcm.Name)
		return false, err
	}

	// clean up the source hub resource -> confirmationEvent: database items change into MigrationCompleted
	// BootstrapSecret, klusterletConfig, detaching clusters
	var deployed []models.ManagedClusterMigration
	err = database.GetGorm().Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.ConditionTypeDeployed,
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
		// Deprecated
		log.Infof("cleaning up the source hub resources: %s", sourceHub)
		err = m.sendEventToSourceHub(ctx, sourceHub, mcm, migrationv1alpha1.ConditionTypeCleaned,
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

	// Deprecated: clean up resource from destination hub
	if err := m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.ConditionTypeCleaned, nil); err != nil {
		return false, err
	}

	// Deprecated: remove database
	err = database.GetGorm().Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.ConditionTypeCleaned,
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
