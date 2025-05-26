package migration

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	conditionReasonResourcesNotDeployed = "ResourcesNotDeployed"
	conditionReasonResourcesDeployed    = "ResourcesDeployed"
	conditionReasonResourcesDeploying   = "ResourcesDeploying"
)

// Migrating - deploying:
//  1. Source Hub: send the resources to migration topic
//  2. Destination Hub: start consume message from migration topic:
//     - apply resources into current hub
//     - report the confirmation for the resources applying
func (m *ClusterMigrationController) deploying(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseDeploying {
		return false, nil
	}
	log.Info("migration deploying")

	condType := migrationv1alpha1.ConditionTypeDeployed
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonResourcesDeployed
	condMessage := "Resources have been successfully deployed to the target hub cluster"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonResourcesNotDeployed
		}
		log.Debugf("deploying condition %s(%s): %s", condType, condReason, condMessage)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage, migrationStageTimeout)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}
	}()

	errMessage := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying)
	if errMessage != "" {
		err = fmt.Errorf("deploying to hub %s error: %s", mcm.Spec.To, errMessage)
		return true, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying) {
		condMessage = fmt.Sprintf("The resources is deploying into the target hub %s", mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		condReason = conditionReasonResourcesDeploying
		return true, nil
	}

	log.Info("migration deploying finished")
	return false, nil
}
