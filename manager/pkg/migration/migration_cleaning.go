package migration

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	ConditionReasonResourceCleaned = "ResourceCleaned"
)

// cleaning handles the cleanup phase of migration
// 1. Once the cluster registered into the destination hub, Clean up the resources in the source hub
// 2. Clean up the resource if the migration failed, to let it rollback
func (m *ClusterMigrationController) cleaning(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseCleaning {
		return false, nil
	}

	log.Info("migration start cleaning")

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeCleaned,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: "Waiting for the resources to be cleaned up from the both source and target hub clusters",
	}

	nextPhase := migrationv1alpha1.PhaseCleaning

	defer m.handleCleaningStatus(ctx, mcm, &condition, &nextPhase, migratingTimeout)

	// Deleting the ManagedServiceAccount will revoke the bootstrap kubeconfig secret of the migrated cluster.
	// Be cautious — this action may carry potential risks.
	if err := m.deleteManagedServiceAccount(ctx, mcm); err != nil {
		log.Errorf("failed to delete the managedServiceAccount: %s/%s", mcm.Spec.To, mcm.Name)
		condition.Message = fmt.Sprintf("failed to delete managedServiceAccount: %v", err)
		condition.Reason = ConditionReasonError
		return false, nil // Let defer handle the status update
	}

	// cleanup the source hub: cleaning or failed state
	fromHub := mcm.Spec.From
	cleaningClusters := GetClusterList(string(mcm.UID))

	// only cleaning the ready clusters for registering phase
	if m.determineFailedStage(ctx, mcm) == migrationv1alpha1.PhaseRegistering {
		registeringReadyClusters := GetReadyClusters(string(mcm.UID), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
		if len(registeringReadyClusters) > 0 {
			log.Infof("cleaning the registering ready clusters: %v", registeringReadyClusters)
			cleaningClusters = registeringReadyClusters
		}
	}

	bootstrapSecret := getBootstrapSecret(mcm.Spec.To, nil)
	if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseCleaning) {
		if err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseCleaning, cleaningClusters,
			bootstrapSecret, ""); err != nil {
			condition.Message = fmt.Sprintf("failed to send cleanup event to source hub %s: %v", fromHub, err)
			condition.Reason = ConditionReasonError
			return false, nil // Let defer handle the status update
		}
		SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseCleaning)
	}

	// cleanup the target hub: cleaning or failed state
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning) {
		if err := m.sendEventToTargetHub(ctx, mcm, mcm.Status.Phase, cleaningClusters, ""); err != nil {
			condition.Message = fmt.Sprintf("failed to send cleanup event to destination hub %s: %v", mcm.Spec.To, err)
			condition.Reason = ConditionReasonError
			return false, nil // Let defer handle the status update
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning)
	}

	// check error message
	errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning)
	if errMsg != "" {
		condition.Message = fmt.Sprintf("cleaning target hub %s with err :%s", mcm.Spec.To, errMsg)
		condition.Reason = ConditionReasonError
		return false, nil
	}
	errMsg = GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseCleaning)
	if errMsg != "" {
		condition.Message = fmt.Sprintf("cleaning source hub %s with err :%s", fromHub, errMsg)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning) {
		condition.Message = fmt.Sprintf("The target hub %s is cleaning", mcm.Spec.To)
		return true, nil
	}

	if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseCleaning) {
		condition.Message = fmt.Sprintf("The source hub %s is cleaning", fromHub)
		return true, nil
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonResourceCleaned
	condition.Message = "Resources have been successfully cleaned up from the hub clusters"
	nextPhase = migrationv1alpha1.PhaseCompleted

	log.Info("migration cleaning finished")
	return false, nil
}

func (m *ClusterMigrationController) deleteManagedServiceAccount(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration,
) error {
	msa := &v1beta1.ManagedServiceAccount{}
	if err := m.Get(ctx, types.NamespacedName{
		Name:      migration.Name,
		Namespace: migration.Spec.To,
	}, msa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get the managedservieaccount: %v", err)
	}
	return m.Delete(ctx, msa)
}

// handleCleaningStatus updates the migration status for cleaning phase
// Cleaning phase always transitions to Completed regardless of success or failure
func (m *ClusterMigrationController) handleCleaningStatus(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition *metav1.Condition,
	nextPhase *string,
	stageTimeout time.Duration,
) {
	startedCond := meta.FindStatusCondition(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeStarted)

	// Handle timeout - convert to warning and set Completed status
	if condition.Reason == ConditionReasonWaiting && startedCond != nil &&
		time.Since(startedCond.LastTransitionTime.Time) > stageTimeout {
		condition.Reason = ConditionReasonTimeout
		condition.Message = fmt.Sprintf("Cleanup timeout: %s", condition.Message)
		condition.Status = metav1.ConditionFalse
	}

	// Handle errors - convert to warning and set Completed status
	if condition.Reason == ConditionReasonError {
		condition.Message = fmt.Sprintf("[Warning - Cleanup Issues] %s.", condition.Message)
		condition.Status = metav1.ConditionFalse
	}

	if condition.Reason != ConditionReasonWaiting {
		// Ensure cleaning always ends in Completed phase, regardless of condition status
		*nextPhase = migrationv1alpha1.PhaseCompleted
	}

	err := m.UpdateStatusWithRetry(ctx, mcm, *condition, *nextPhase)
	if err != nil {
		log.Errorf("failed to update the %s condition: %v", condition.Type, err)
	}
}
