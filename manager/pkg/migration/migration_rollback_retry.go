package migration

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// handleRollbackRetryRequest checks if a failed migration has the rollback retry annotation
// and resets it to PhaseRollbacking if so. This allows users to manually trigger a rollback
// retry for migrations that failed during the rollback phase.
func (m *ClusterMigrationController) handleRollbackRetryRequest(ctx context.Context,
	req ctrl.Request,
) (bool, error) {
	mcm := &migrationv1alpha1.ManagedClusterMigration{}
	if err := m.Get(ctx, req.NamespacedName, mcm); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Only process failed migrations with the rollback retry annotation
	if mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
		return false, nil
	}

	annotations := mcm.GetAnnotations()
	if annotations == nil {
		return false, nil
	}

	requestValue, exists := annotations[constants.MigrationRequestAnnotationKey]
	if !exists || requestValue != constants.MigrationRollbackAnnotationValue {
		return false, nil
	}
	// Ensure migration status exists in memory cache before operations
	AddMigrationStatus(string(mcm.GetUID()))

	log.Infof("rollback retry requested for migration %s (uid: %s)", mcm.Name, mcm.UID)

	// Before retrying rollback, query the target hub for current cluster status and update ConfigMap
	if err := m.queryAndUpdateMigrationClusterResults(ctx, mcm); err != nil {
		log.Warnf("failed to query and update migration cluster results before retry: %v", err)
		// Continue with retry even if update fails - the rollback itself is more important
	}

	// Remove annotation and update status in a single retry loop
	if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		// Remove the annotation to prevent infinite retry loops
		annotations := mcm.GetAnnotations()
		delete(annotations, constants.MigrationRequestAnnotationKey)
		mcm.SetAnnotations(annotations)
		if err := m.Update(ctx, mcm); err != nil {
			return err
		}
		// Remove the existing RolledBack and Cleaned conditions to reset LastTransitionTime
		// Cleaned condition must also be removed because rollback can transition to cleaning phase
		// and the old True status would cause cleaning() to return early
		meta.RemoveStatusCondition(&mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRolledBack)
		meta.RemoveStatusCondition(&mcm.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned)
		// Add new condition with fresh LastTransitionTime
		meta.SetStatusCondition(&mcm.Status.Conditions, metav1.Condition{
			Type:               migrationv1alpha1.ConditionTypeRolledBack,
			Status:             metav1.ConditionFalse,
			Reason:             ConditionReasonWaiting,
			Message:            "Rollback retry requested via annotation",
			LastTransitionTime: metav1.NewTime(time.Now()),
		})
		mcm.Status.Phase = migrationv1alpha1.PhaseRollbacking
		return m.Status().Update(ctx, mcm)
	}); err != nil {
		return false, fmt.Errorf("failed to reset migration to rollbacking phase: %w", err)
	}

	// Reset the rollback and cleaning stage states in memory cache to allow re-execution
	// Both phases need to be reset because rollback can transition to cleaning if success clusters exist
	ResetStageState(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseRollbacking)
	ResetStageState(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRollbacking)
	ResetStageState(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseCleaning)
	ResetStageState(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning)

	log.Infof("migration %s reset to rollbacking phase for retry", mcm.Name)
	return true, nil
}

// queryAndUpdateMigrationClusterResults sends a query event to the target hub to get the current
// migration status of clusters, waits for the response, and updates the ConfigMap with the results.
// This is used before retrying rollback to determine which clusters have successfully migrated
// (ManifestWork ready) and which have not.
func (m *ClusterMigrationController) queryAndUpdateMigrationClusterResults(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	migrationID := string(mcm.UID)
	toHub := mcm.Spec.To
	if err := m.RestoreClusterList(ctx, mcm); err != nil {
		log.Errorf("failed to set clusterList %v", err)
		return err
	}

	// Get all clusters that were being migrated
	allClusters := GetClusterList(migrationID)
	if len(allClusters) == 0 {
		log.Infof("no clusters found in memory cache for migration %s, skipping cluster status query", mcm.Name)
		return nil
	}

	log.Infof("querying migration status from target hub %s for migration %s", toHub, mcm.Name)

	// Send query event to target hub
	if err := m.sendEventToTargetHub(ctx, mcm, migrationv1alpha1.PhaseQueryMigrationStatus, allClusters, ""); err != nil {
		return fmt.Errorf("failed to send query migration status event to target hub: %w", err)
	}
	SetStarted(migrationID, toHub, migrationv1alpha1.PhaseQueryMigrationStatus)

	// Wait for response from target hub with timeout
	queryTimeout := 2 * time.Minute
	pollInterval := 5 * time.Second

	queryCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	err := wait.PollUntilContextTimeout(queryCtx, pollInterval, queryTimeout, true,
		func(context.Context) (done bool, err error) {
			return GetFinished(migrationID, toHub, migrationv1alpha1.PhaseQueryMigrationStatus), nil
		})
	if err != nil {
		return fmt.Errorf("timeout waiting for migration status query response from target hub: %w", err)
	}

	// Get the cluster errors from the query response
	// Clusters in clusterErrors have incomplete migration (ManifestWork not ready)
	clusterErrors := GetClusterErrors(migrationID, toHub, migrationv1alpha1.PhaseQueryMigrationStatus)
	// Calculate success and failure clusters based on error map
	// Success: clusters with ManifestWork ready (not in clusterErrors)
	// Failure: clusters with ManifestWork not ready (in clusterErrors)
	var successClusters, failedClusters []string
	for _, cluster := range allClusters {
		if _, hasError := clusterErrors[cluster]; hasError {
			failedClusters = append(failedClusters, cluster)
		} else {
			successClusters = append(successClusters, cluster)
		}
	}

	log.Infof("migration status query completed for migration %s: success=%d, failed=%d",
		mcm.Name, len(successClusters), len(failedClusters))

	// Update the ConfigMap with success and failure lists
	if err := m.UpdateSuccessAndFailureClustersToConfigMap(ctx, mcm, successClusters, failedClusters); err != nil {
		return fmt.Errorf("failed to store migration cluster results to ConfigMap: %w", err)
	}

	return nil
}
