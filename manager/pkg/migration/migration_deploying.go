package migration

import (
	"context"
	"encoding/json"
	"fmt"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// Migrating - deploying:
//  1. Destination Hub: sync the addon configuration from database,
//  2. Send the applied confirmation event into status MigrationClusterRegistered -> MigrationClusterDeployed
func (m *ClusterMigrationController) deploying(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed) {
		return false, nil
	}

	condType := migrationv1alpha1.ConditionTypeDeployed
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonAddonConfigDeployed
	condMsg := "AddonConfigs have been deployed to all migrated clusters"
	var err error

	defer func() {
		if err != nil {
			condMsg = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonAddonConfigNotDeployed
		}
		log.Infof("deploying condition %s(%s): %s", condType, condReason, condMsg)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMsg)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}
	}()

	log.Info("migration deploying")

	// registeredClusters
	var registeredClusters []models.ManagedClusterMigration
	db := database.GetGorm()
	err = db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.ConditionTypeRegistered,
	}).Find(&registeredClusters).Error
	if err != nil {
		return false, err
	}

	// deploy addonConfigs
	var deploying []models.ManagedClusterMigration
	for _, registered := range registeredClusters {
		klusterletAddonConfig := &addonv1.KlusterletAddonConfig{}
		if err := json.Unmarshal([]byte(registered.Payload), klusterletAddonConfig); err != nil {
			return false, err
		}

		// mark it is deployed if the resource is default value
		defaultAddonConfig := &addonv1.KlusterletAddonConfig{}
		if apiequality.Semantic.DeepDerivative(klusterletAddonConfig.Spec, defaultAddonConfig.Spec) {
			err = db.Model(&models.ManagedClusterMigration{}).
				Where("to_hub = ?", registered.ToHub).
				Where("cluster_name = ?", registered.ClusterName).
				Update("stage", migrationv1alpha1.ConditionTypeDeployed).Error
			if err != nil {
				return false, err
			}
			log.Infof("AddonConfig %s is default(none), skip deploying to %s", registered.ClusterName, registered.ToHub)
			continue
		}

		// send to destination hub to deploy the addonConfig, the status mark it as Completed
		deploying = append(deploying, registered)
		if err = m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.PhaseMigrating,
			klusterletAddonConfig); err != nil {
			return false, err
		}
	}

	// update condition
	if len(deploying) > 0 {
		migratingMessages := []string{}
		for idx, m := range deploying {
			migratingMessages = append(migratingMessages, m.ClusterName)
			// if the migrating clusters more than 3, only show the first 3 items in the condition message
			if idx == 2 {
				migratingMessages = append(migratingMessages, "...")
				break
			}
		}
		condMsg = fmt.Sprintf("Cluster AddonConfigs %v have not been deployed to hub %s", migratingMessages, mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		condReason = conditionReasonAddonConfigNotDeployed
		return true, nil
	}

	return false, nil
}
