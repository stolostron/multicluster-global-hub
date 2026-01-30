package migration

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const ConditionReasonResourceRolledBack = "ResourceRolledBack"

// rollbacking handles the rollback process when migration fails during initializing, deploying, or registering phases
// This phase attempts to restore the system to its previous state before the failed migration attempt
// For registering stage, the flow is:
// 1. Target Hub: query not-ready clusters first and return them, then clean up resources
// 2. Source Hub: restore only the not-ready clusters based on target hub response
// 3. Global Hub: update the CR status and transition to Failed phase
// For other stages, both hubs receive all clusters for rollback.
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

	log.Infof("start rollbacking: %s (uid: %s)", mcm.Name, mcm.UID)

	// Determine the failed stage to provide context in messages
	failedStage := m.determineFailedStage(ctx, mcm)

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeRolledBack,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: fmt.Sprintf("Attempting to rollback migration changes for failed %s stage", failedStage),
	}
	nextPhase := migrationv1alpha1.PhaseRollbacking
	waitingHub := mcm.Spec.To // Start waiting for target hub first

	// Track success clusters calculated during rollback - passed to defer handler to avoid cache issues
	var successClusters []string

	defer m.handleRollbackStatus(ctx, mcm, &condition, &nextPhase, &waitingHub, failedStage, &successClusters)

	fromHub := mcm.Spec.From
	toHub := mcm.Spec.To
	migrationID := string(mcm.UID)
	allClusters := GetClusterList(migrationID)

	if len(allClusters) == 0 {
		condition.Message = fmt.Sprintf("No clusters to rollback for %s stage", failedStage)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	// 1. Send rollback event to target hub FIRST to query not-ready clusters and clean up
	if !GetStarted(migrationID, toHub, migrationv1alpha1.PhaseRollbacking) {
		err := m.sendEventToTargetHub(ctx, mcm, migrationv1alpha1.PhaseRollbacking, allClusters, failedStage)
		if err != nil {
			condition.Message = fmt.Sprintf("Failed to send %s stage rollback event to target hub %s: %v",
				failedStage, toHub, err)
			condition.Reason = ConditionReasonError
			return false, err
		}
		log.Infof("rollbacking to target hub(%s): %s (uid: %s)", toHub, mcm.Name, mcm.UID)
		SetStarted(migrationID, toHub, migrationv1alpha1.PhaseRollbacking)
	}

	// 2. For registering stage, must wait for FailedClustersReported result from target hub
	// This allows source hub rollback to run in parallel with target hub rollback
	rollbackingClusters := allClusters
	if failedStage == migrationv1alpha1.PhaseRegistering {
		// Must wait for failed clusters report from target hub
		if !HasFailedClustersReported(migrationID, toHub, migrationv1alpha1.PhaseRollbacking) {
			// Still waiting for failed clusters list from target hub
			condition.Message = fmt.Sprintf("Waiting for target hub %s to report failed clusters", toHub)
			waitingHub = toHub
			setRetry(mcm, migrationv1alpha1.PhaseRollbacking, migrationv1alpha1.ConditionTypeRolledBack, toHub)
			return true, nil
		}

		// Got the failed clusters report from target hub
		failedClusters := GetFailedClusters(migrationID, toHub, migrationv1alpha1.PhaseRollbacking)
		rollbackingClusters = failedClusters

		// Calculate success clusters and update ConfigMap
		successClusters = m.CalculateAndUpdateClusterResults(ctx, mcm, allClusters, failedClusters)

		// If no failed clusters, all clusters migrated successfully - exit rollback
		if len(failedClusters) == 0 {
			log.Infof("no failed clusters reported - all clusters migrated successfully, skipping rollback")
			condition.Status = metav1.ConditionTrue
			condition.Reason = ConditionReasonResourceRolledBack
			condition.Message = fmt.Sprintf("%s rollback completed - all clusters migrated successfully. "+
				"Check configmap %q to get detailed migrated cluster list.", failedStage, mcm.Name)
			return false, nil
		}
	}

	log.Infof("rollbacking clusters on source hub: %v", rollbackingClusters)

	// 3. Send rollback event to source hub with the clusters that need rollback
	// For registering stage, this runs in parallel with target hub rollback
	if !GetStarted(migrationID, fromHub, migrationv1alpha1.PhaseRollbacking) {
		err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseRollbacking, rollbackingClusters,
			getBootstrapSecret(fromHub, nil), failedStage)
		if err != nil {
			condition.Message = fmt.Sprintf("Failed to send %s stage rollback event to source hub %s: %v",
				failedStage, fromHub, err)
			condition.Reason = ConditionReasonError
			return false, err
		}
		log.Infof("rollbacking to source hub(%s): %s (uid: %s)", fromHub, mcm.Name, mcm.UID)
		SetStarted(migrationID, fromHub, migrationv1alpha1.PhaseRollbacking)
	}

	// Check for rollback errors from source hubs
	if errMsg := GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking); errMsg != "" {
		m.handleErrorList(mcm, fromHub, migrationv1alpha1.PhaseRollbacking)
		condition.Message = m.manuallyRollbackMsg(failedStage, fromHub, errMsg)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	// Check for rollback errors from destination hub
	if errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking); errMsg != "" {
		m.handleErrorList(mcm, mcm.Spec.To, migrationv1alpha1.PhaseRollbacking)

		condition.Message = fmt.Sprintf("%s stage rollback failed on target hub %s: %s", failedStage, mcm.Spec.To, errMsg)
		condition.Reason = ConditionReasonError
		condition.Status = metav1.ConditionFalse
		return false, nil
	}

	// Wait for source hub rollback completion
	if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRollbacking) {
		condition.Message = fmt.Sprintf("Waiting for source hub %s to complete %s stage rollback", fromHub, failedStage)
		waitingHub = fromHub
		setRetry(mcm, migrationv1alpha1.PhaseRollbacking, migrationv1alpha1.ConditionTypeRolledBack, fromHub)
		return true, nil
	}

	// Wait for destination hub rollback completion
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking) {
		condition.Message = fmt.Sprintf("Waiting for target hub %s to complete %s stage rollback", mcm.Spec.To, failedStage)
		waitingHub = mcm.Spec.To
		setRetry(mcm, migrationv1alpha1.PhaseRollbacking, migrationv1alpha1.ConditionTypeRolledBack, mcm.Spec.To)
		return true, nil
	}

	// 3. Clean up managed service account and related resources created during initialization
	if err := m.cleanupManagedServiceAccount(ctx, mcm); err != nil {
		condition.Message = fmt.Sprintf("Failed to cleanup managed service account: %v", err)
		condition.Reason = ConditionReasonError
		return false, err
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonResourceRolledBack
	condition.Message = fmt.Sprintf("%s rollback %d clusters completed successfully.",
		failedStage, len(rollbackingClusters))

	log.Infof("finish rollbacking: %s (uid: %s)", mcm.Name, mcm.UID)
	return false, nil
}

func (m *ClusterMigrationController) manuallyRollbackMsg(failedStage, fromHub, errMsg string) string {
	return fmt.Sprintf("%s stage rollback failed on hub %s: %s. "+
		"Manual intervention required: please ensure annotations (%s and %s) are removed from the managed clusters, "+
		"and remove annotation %s from clusterdeployment",
		failedStage, fromHub, errMsg, constants.ManagedClusterMigrating,
		"agent.open-cluster-management.io/klusterlet-config",
		"hive.openshift.io/reconcile-pause")
}

// handleRollbackStatus updates the migration status for rollback phase
// Rollback phase always transitions to Failed phase regardless of success or failure
func (m *ClusterMigrationController) handleRollbackStatus(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition *metav1.Condition,
	nextPhase *string,
	waitingHub *string,
	failedStage string,
	successClusters *[]string,
) {
	_ = updateConditionWithTimeout(mcm, condition, getTimeout(migrationv1alpha1.PhaseRollbacking),
		m.manuallyRollbackMsg(failedStage, *waitingHub, "Timeout"))
	// means the rollback is finished whether it's successful or failed
	if condition.Reason != ConditionReasonWaiting {
		*nextPhase = migrationv1alpha1.PhaseFailed
		log.Infof("migration rollbacking finished for failed %s stage", failedStage)
		// if registering, cleaning the ready clusters
		if failedStage == migrationv1alpha1.PhaseRegistering {
			// Use success clusters calculated during rollback if available (avoids cache issues)
			if successClusters != nil && len(*successClusters) > 0 {
				*nextPhase = migrationv1alpha1.PhaseCleaning
				log.Infof("success clusters found %d, cleaning the ready clusters", len(*successClusters))
			}
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
