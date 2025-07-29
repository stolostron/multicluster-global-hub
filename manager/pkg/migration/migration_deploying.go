package migration

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	ConditionReasonResourcesDeployed = "ResourcesDeployed"
)

// Migrating - deploying:
//  1. Source Hub: send the resources to migration topic
//  2. Destination Hub: start consume message from migration topic:
//     - apply resources into current hub
//     - report the confirmation for the resources applying
func (m *ClusterMigrationController) deploying(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseDeploying {
		return false, nil
	}
	log.Info("migration deploying")

	condType := migrationv1alpha1.ConditionTypeDeployed
	condStatus := metav1.ConditionTrue
	condReason := ConditionReasonResourcesDeployed
	condMessage := "Resources have been successfully deployed to the target hub cluster"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = "ResourcesNotDeployed"
		}
		log.Debugf("deploying condition %s(%s): %s", condType, condReason, condMessage)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage, migrationStageTimeout)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}
	}()

	// check the source hub to see is there any error message reported
	sourceHubToClusters := GetSourceClusters(string(mcm.GetUID()))
	if sourceHubToClusters == nil {
		err = fmt.Errorf("not initialized the source clusters for migrationId: %s", string(mcm.GetUID()))
		return false, nil
	}

	for fromHub, clusters := range sourceHubToClusters {
		if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying) {
			log.Infof("migration deploying to source hub: %s", fromHub)
			err = m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseDeploying, clusters,
				nil, nil)
			if err != nil {
				return false, nil
			}
			SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying)
		}

		errMessage := GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying)
		if errMessage != "" {
			err = fmt.Errorf("deploying source hub %s error: %s", fromHub, errMessage)
			return false, nil
		}

		// waiting the source hub confirmation
		if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying) {
			condMessage = fmt.Sprintf("waiting for resources to be prepared in the source hub %s", fromHub)
			condStatus = metav1.ConditionFalse
			condReason = ConditionReasonWaiting
			err = nil // waiting should not be considered an error
			return true, nil
		}
	}

	// check the target hub to see is there any error message reported
	errMessage := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying)
	if errMessage != "" {
		err = fmt.Errorf("deploying to hub %s error: %s", mcm.Spec.To, errMessage)
		return false, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying) {
		condMessage = fmt.Sprintf("waiting for resources to be deployed into the target hub %s", mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonWaiting
		err = nil // waiting should not be considered an error
		return true, nil
	}

	condStatus = metav1.ConditionTrue
	condReason = ConditionReasonResourcesDeployed
	condMessage = "Resources have been successfully deployed to the target hub cluster"

	log.Info("migration deploying finished")
	return false, nil
}
