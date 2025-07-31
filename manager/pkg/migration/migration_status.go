package migration

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// UpdateConditionWithRetry updates the migration's condition with retry logic and timeout handling
func (m *ClusterMigrationController) UpdateConditionWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
	timeout time.Duration,
) error {
	// The MigrationStarted type should not have timeout
	if conditionType != migrationv1alpha1.ConditionTypeStarted &&
		status == metav1.ConditionFalse && time.Since(mcm.CreationTimestamp.Time) > timeout {
		reason = ConditionReasonTimeout
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Client.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		if err := m.UpdateCondition(ctx, mcm, conditionType, status, reason, message); err != nil {
			return err
		}
		return nil
	})
}

// UpdateCondition updates the migration's condition and handles phase transitions
func (m *ClusterMigrationController) UpdateCondition(
	ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	update := meta.SetStatusCondition(&mcm.Status.Conditions, newCondition)

	// Handle phase updates based on condition type and status
	if status == metav1.ConditionFalse {
		// migration not start, means the migration is pending
		if conditionType == migrationv1alpha1.ConditionTypeStarted && mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
			mcm.Status.Phase = migrationv1alpha1.PhasePending
			update = true
		}

		// Check if migration should go to cleaning phase before failed
		// Enter failure handling if validation failed OR reason is not waiting
		if conditionType == migrationv1alpha1.ConditionTypeValidated || reason != ConditionReasonWaiting {
			// Check if cleanup is needed before setting to failed
			if shouldCleanupBeforeFailed(mcm) {
				mcm.Status.Phase = migrationv1alpha1.PhaseCleaning
				update = true
			} else {
				mcm.Status.Phase = migrationv1alpha1.PhaseFailed
				update = true
			}
		}
	} else {
		switch conditionType {
		case migrationv1alpha1.ConditionTypeValidated:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseInitializing
			}
		case migrationv1alpha1.ConditionTypeInitialized:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseDeploying
			}
		case migrationv1alpha1.ConditionTypeDeployed:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseRegistering
			}
		case migrationv1alpha1.ConditionTypeRegistered:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseCleaning
			}
		case migrationv1alpha1.ConditionTypeCleaned:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted && mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
				// If ConditionTypeRegistered is True, it means normal cleanup after successful registration
				// Otherwise, it's cleanup after failure
				if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered) {
					mcm.Status.Phase = migrationv1alpha1.PhaseCompleted
				} else {
					mcm.Status.Phase = migrationv1alpha1.PhaseFailed
				}
			}
		}
		update = true
	}

	// Update the resource in Kubernetes only if there was a change
	if update {
		return m.Status().Update(ctx, mcm)
	}
	return nil
}

// getSourceClusters returns a map of source hub names to their managed clusters
func getSourceClusters(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string][]string, error) {
	db := database.GetGorm()
	managedClusterMap := make(map[string][]string)
	if mcm.Spec.From != "" {
		managedClusterMap[mcm.Spec.From] = mcm.Spec.IncludedManagedClusters
		return managedClusterMap, nil
	}

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name FROM status.managed_clusters
			WHERE cluster_name IN (?)`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to get leaf hub name and managed clusters - %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var leafHubName, managedClusterName string
		if err := rows.Scan(&leafHubName, &managedClusterName); err != nil {
			return nil, fmt.Errorf("failed to scan leaf hub name and managed cluster name - %w", err)
		}
		// If the cluster is synced into the to hub, ignore it.
		if leafHubName == mcm.Spec.To {
			continue
		}
		managedClusterMap[leafHubName] = append(managedClusterMap[leafHubName], managedClusterName)
	}
	return managedClusterMap, nil
}

// shouldCleanupBeforeFailed checks if the migration needs cleanup before being marked as failed
func shouldCleanupBeforeFailed(mcm *migrationv1alpha1.ManagedClusterMigration) bool {
	// Only need cleanup if migration failed during initializing, deploying, or registering phases
	return mcm.Status.Phase == migrationv1alpha1.PhaseInitializing ||
		mcm.Status.Phase == migrationv1alpha1.PhaseDeploying ||
		mcm.Status.Phase == migrationv1alpha1.PhaseRegistering
}