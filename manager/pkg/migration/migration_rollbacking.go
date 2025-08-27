package migration

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	ConditionReasonResourceRolledBack = "ResourceRolledBack"
	ConditionReasonRollbackFailed     = "RollbackFailed"
)

// rollbacking handles the rollback process when migration fails during initializing, deploying, or registering phases
// This phase attempts to restore the system to its previous state before the failed migration attempt
// 1. Source Hub: restore original cluster configurations and remove migration-related changes
// 2. Destination Hub: clean up any partially created resources and configurations
// 3. Global Hub: update the CR status and transition to Failed phase
func (m *ClusterMigrationController) rollbacking(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRolledBack) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseRollbacking {
		return false, nil
	}

	log.Infof("migration %s rollbacking started", mcm.Name)

	// Determine the failed stage to provide context in messages
	failedStage := m.determineFailedStage(ctx, mcm)

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeRolledBack,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: fmt.Sprintf("Attempting to rollback migration changes for failed %s stage", failedStage),
	}
	nextPhase := migrationv1alpha1.PhaseRollbacking
	waitingHub := mcm.Spec.From

	defer m.handleRollbackStatus(ctx, mcm, &condition, &nextPhase, &waitingHub, failedStage)

	// 1. Send rollback events to source hubs to restore original configurations
	fromHub := mcm.Spec.From
	clusters := GetClusterList(string(mcm.UID))

	if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking) {
		log.Infof("sending rollback event to source hub: %s", fromHub)
		err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseRollbacking, clusters,
			getBootstrapSecret(fromHub, nil), failedStage)
		if err != nil {
			condition.Message = fmt.Sprintf("failed to send %s stage rollback event to source hub %s: %v",
				failedStage, fromHub, err)
			condition.Reason = ConditionReasonRollbackFailed
			return false, err
		}
		SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking)
	}

	// Check for rollback errors from source hubs
	if errMsg := GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking); errMsg != "" {
		// Only add manual cleanup guidance for source hub rollback failures
		condition.Message = m.manuallyRollbackMsg(failedStage, fromHub, errMsg)
		condition.Reason = ConditionReasonRollbackFailed
		return false, nil
	}

	// Wait for source hub rollback completion
	if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking) {
		condition.Message = fmt.Sprintf("waiting for source hub %s to complete %s stage rollback", fromHub, failedStage)
		waitingHub = fromHub
		return true, nil
	}

	// 2. Send rollback event to destination hub to clean up partial resources
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking) {
		log.Infof("sending rollback event to destination hub: %s", mcm.Spec.To)
		err := m.sendEventToTargetHub(ctx, mcm, migrationv1alpha1.PhaseRollbacking, clusters, failedStage)
		if err != nil {
			condition.Message = fmt.Sprintf("failed to send %s stage rollback event to target hub %s: %v", failedStage,
				mcm.Spec.To, err)
			condition.Reason = ConditionReasonRollbackFailed
			condition.Status = metav1.ConditionFalse
			return false, err
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking)
	}

	// Check for rollback errors from destination hub
	if errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking); errMsg != "" {
		condition.Message = fmt.Sprintf("%s stage rollback failed on target hub %s: %s", failedStage, mcm.Spec.To, errMsg)
		condition.Reason = ConditionReasonRollbackFailed
		condition.Status = metav1.ConditionFalse
		return false, nil
	}

	// Wait for destination hub rollback completion
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking) {
		condition.Message = fmt.Sprintf("waiting for target hub %s to complete %s stage rollback", mcm.Spec.To, failedStage)
		waitingHub = mcm.Spec.To
		return true, nil
	}

	// 3. Clean up managed service account and related resources created during initialization
	if err := m.cleanupManagedServiceAccount(ctx, mcm); err != nil {
		condition.Message = fmt.Sprintf("failed to cleanup managed service account: %v", err)
		condition.Reason = ConditionReasonRollbackFailed
		return false, err
	}

	// 4. Clean up migration annotations from managed clusters on source hubs
	// Note: This is handled by the rollback events sent to source hubs above
	log.Info("managed cluster annotation cleanup will be handled by source hub agents")

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonResourceRolledBack
	condition.Message = fmt.Sprintf("%s rollback completed successfully.", failedStage)
	nextPhase = migrationv1alpha1.PhaseFailed

	log.Info("migration rollbacking finished - transitioning to Failed")
	return false, nil
}

func (m *ClusterMigrationController) manuallyRollbackMsg(failedStage, fromHub, errMsg string) string {
	return fmt.Sprintf("%s stage rollback failed on source hub %s: %s. "+
		"Manual intervention required: please remove migration annotations (%s and %s) from the managed clusters",
		failedStage, fromHub, errMsg, constants.ManagedClusterMigrating, "agent.open-cluster-management.io/klusterlet-config")
}

// handleRollbackStatus updates the migration status for rollback phase
// Rollback phase always transitions to Failed phase regardless of success or failure
func (m *ClusterMigrationController) handleRollbackStatus(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition *metav1.Condition,
	nextPhase *string,
	waitingHub *string,
	failedStage string,
) {
	startedCond := meta.FindStatusCondition(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeStarted)
	// Handle timeout - still transition to Failed but with timeout message
	if condition.Reason == ConditionReasonWaiting && startedCond != nil &&
		time.Since(startedCond.LastTransitionTime.Time) > (getTimeout(failedStage)+getTimeout(migrationv1alpha1.PhaseRollbacking)) {
		condition.Reason = ConditionReasonTimeout
		condition.Message = m.manuallyRollbackMsg(failedStage, *waitingHub, "Timeout")
	}

	if condition.Reason != ConditionReasonWaiting {
		*nextPhase = migrationv1alpha1.PhaseFailed
		if m.EventRecorder != nil {
			m.EventRecorder.Eventf(mcm, corev1.EventTypeWarning, condition.Reason, condition.Message)
		}
	}

	err := m.UpdateStatusWithRetry(ctx, mcm, *condition, *nextPhase)
	if err != nil {
		log.Errorf("failed to update the %s condition: %v", condition.Type, err)
	}
}

// cleanupManagedServiceAccount removes the managed service account created during initialization
func (m *ClusterMigrationController) cleanupManagedServiceAccount(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	// This will be handled by the existing cleanup logic when the MSA is deleted
	// The controller watches for MSA deletions and will trigger cleanup
	log.Infof("managed service account cleanup will be handled by existing deletion logic for migration %s", mcm.Name)
	return nil
}

// determineFailedStage determines which stage failed by checking the migration conditions
func (m *ClusterMigrationController) determineFailedStage(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) string {
	// Check conditions to determine which stage failed
	for _, condition := range mcm.Status.Conditions {
		if condition.Status == metav1.ConditionFalse {
			switch condition.Type {
			case migrationv1alpha1.ConditionTypeInitialized:
				return migrationv1alpha1.PhaseInitializing
			case migrationv1alpha1.ConditionTypeDeployed:
				return migrationv1alpha1.PhaseDeploying
			case migrationv1alpha1.ConditionTypeRegistered:
				return migrationv1alpha1.PhaseRegistering
			}
		}
	}

	// Default to current phase if we can't determine the failed stage
	return mcm.Status.Phase
}
