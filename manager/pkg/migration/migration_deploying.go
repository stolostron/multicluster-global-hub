package migration

import (
	"context"
	"encoding/json"
	"fmt"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
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
	// skip if the phase isn't Migrating and the MigrationResourceDeployed condition is True
	if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating &&
		meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		return false, nil
	}

	condType := migrationv1alpha1.MigrationResourceDeployed
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

	// registered
	var registered []models.ManagedClusterMigration
	db := database.GetGorm()
	err = db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationClusterRegistered,
	}).Find(&registered).Error
	if err != nil {
		return false, err
	}

	// deploy addonConfigs
	for _, deploy := range registered {
		klusterAddonConfig := &addonv1.KlusterletAddonConfig{}
		if err := json.Unmarshal([]byte(deploy.Payload), klusterAddonConfig); err != nil {
			return false, err
		}

		if err = m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.PhaseMigrating, klusterAddonConfig); err != nil {
			return false, err
		}
	}

	// update condition
	if len(registered) > 0 {
		migratingMessages := []string{}
		for idx, m := range registered {
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
