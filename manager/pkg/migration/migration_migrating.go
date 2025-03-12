package migration

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	conditionReasonClusterMigrated        = "ClusterMigrated"
	conditionReasonClusterNotRegistered   = "ClusterNotRegistered"
	conditionReasonAddonConfigNotDeployed = "AddonConfigNotDeployed"
)

// Migrating:
//  1. From Hub: register the cluster into To Hub
//  2. To Hub: deploy the addon config into the current Hub
func (m *ClusterMigrationController) migrating(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	// skip if the phase isn't Migrating and the MigrationResourceDeployed condition is True
	if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating &&
		meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		return false, nil
	}

	// To From: select the initialized items to start registering
	var migrations []models.ManagedClusterMigration
	db := database.GetGorm()
	err := db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationResourceInitialized,
	}).Find(&migrations).Error
	if err != nil {
		return false, err
	}

	// check if the cluster is synced to "To Hub"
	// if not, sent registering event to "From hub", else change the item into registered
	fromHubToClusters := map[string][]string{}
	for _, m := range migrations {
		cluster := models.ManagedCluster{}
		err := db.Where("payload->'metadata'->>'name' = ?", m.ClusterName).First(&cluster).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) { // error
			return false, err
		} else if errors.Is(err, gorm.ErrRecordNotFound) { // not found
			log.Info("cluster(%s) is not found in the table", m.ClusterName)
		} else { // found
			if cluster.LeafHubName == m.ToHub { // synced into target hub
				log.Info("cluster(%s) is switched to hub(%s) in the table", m.ClusterName, m.ToHub)
				err := db.Where(&models.ManagedClusterMigration{
					ClusterName: m.ClusterName,
					ToHub:       m.ToHub, FromHub: m.FromHub,
				}).Update("stage", migrationv1alpha1.MigrationClusterRegistered).Error
				if err != nil {
					return false, err
				}
				continue
			} else {
				log.Info("cluster(%s) is not switched into hub(%s)", m.ClusterName, m.ToHub)
			}
		}

		fromHubToClusters[m.FromHub] = append(fromHubToClusters[m.FromHub], m.ClusterName)
	}

	if len(fromHubToClusters) > 0 {
		// update the condition
		notRegisterClusters := []string{}
		for fromHub, clusters := range fromHubToClusters {
			notRegisterClusters = append(notRegisterClusters, clusters...)
			err = m.specToFromHub(ctx, fromHub, migrationv1alpha1.PhaseMigrating, clusters, nil, nil)
			if err != nil {
				return false, err
			}
		}

		// truncate the cluster list if it has more than 3 clusters
		message := ""
		if len(notRegisterClusters) > 3 {
			message = fmt.Sprintf("The clusters [%s, %s, %s, ...] are not registered into hub(%s)",
				notRegisterClusters[0], notRegisterClusters[1], notRegisterClusters[2], mcm.Spec.To)
		} else {
			message = fmt.Sprintf("The clusters %v are not registered into hub(%s)", notRegisterClusters, mcm.Spec.To)
		}

		err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationClusterRegistered,
			metav1.ConditionFalse, conditionReasonClusterNotRegistered, message)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	// if items  deployed

	// conditionReasonAddonConfigNotDeployed
	// err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationClusterRegistered,
	// 	metav1.ConditionFalse, conditionReasonClusterNotRegistered,
	// 	fmt.Sprintf("All the clusters have been registered into hub %s", mcm.Spec.To))
	// if err != nil {
	// 	return false, err
	// }

	// migrated
	err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationResourceDeployed,
		metav1.ConditionTrue, conditionReasonClusterMigrated, "All the clusters are migrated into the new hub")
	if err != nil {
		return false, err
	}
	return false, nil
}
