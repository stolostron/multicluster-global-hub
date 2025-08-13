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

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeDeployed,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: "Waiting for the resources to be deployed into the target hub cluster",
	}
	nextPhase := migrationv1alpha1.PhaseDeploying

	defer m.handleStatusWithRollback(ctx, mcm, &condition, &nextPhase, migrationStageTimeout)

	fromHub := mcm.Spec.From
	clusters := mcm.Spec.IncludedManagedClusters

	// 1. source hub: start and wait the confirmation
	if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying) {
		log.Infof("migration deploying to source hub: %s", fromHub)
		err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseDeploying, clusters, nil, "")
		if err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying)
	}

	errMessage := GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying)
	if errMessage != "" {
		condition.Message = fmt.Sprintf("deploying source hub %s error: %s", fromHub, errMessage)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	// waiting the source hub confirmation
	if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseDeploying) {
		condition.Message = fmt.Sprintf("waiting for resources to be prepared in the source hub %s", fromHub)
		return true, nil
	}

	// 2. target hub: check the confirmation and error message
	errMessage = GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying)
	if errMessage != "" {
		condition.Message = fmt.Sprintf("deploying to hub %s error: %s", mcm.Spec.To, errMessage)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseDeploying) {
		condition.Message = fmt.Sprintf("waiting for resources to be deployed into the target hub %s", mcm.Spec.To)
		return true, nil
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonResourcesDeployed
	condition.Message = "Resources have been successfully deployed to the target hub cluster"
	nextPhase = migrationv1alpha1.PhaseRegistering

	log.Info("migration deploying finished")
	return false, nil
}
