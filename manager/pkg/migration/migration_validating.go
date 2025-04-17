package migration

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	ConditionReasonClusterNotFound   = "ClusterNotFound"
	ConditionReasonResourceValidated = "ResourceValidated"
)

// Validation:
// 1. Verify if the migrating clusters exist in the current hubs
//   - If the clusters do not exist in both the from and to hubs, report a validating error and mark status as failed
//   - If the clusters exist in the destination hub, mark the status as completed.
//   - If the clusters exist in the source hub, mark the status as validated and change the phase to initializing.
func (m *ClusterMigrationController) validating(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if mcm.Status.Phase == "" {
		mcm.Status.Phase = migrationv1alpha1.PhaseValidating
		if err := m.Client.Status().Update(ctx, mcm); err != nil {
			return false, err
		}
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeValidated) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseValidating {
		return false, nil
	}
	log.Info("migration validating")

	condType := migrationv1alpha1.ConditionTypeValidated
	condStatus := metav1.ConditionTrue
	condReason := ConditionReasonResourceValidated
	condMessage := "Migration resources have been validated"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
		}
		log.Infof("validating condition %s(%s): %s", condType, condReason, condMessage)
		e := m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage)
		if e != nil {
			log.Errorf("failed to update the %s condition: %v", condType, e)
		}
	}()

	// cluster: hub in database
	clusterWithHub, err := getClusterWithHub(mcm)

	notFoundClusters := []string{}
	movedClusters := []string{}
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		hub := clusterWithHub[cluster]
		if hub == mcm.Spec.To {
			movedClusters = append(movedClusters, cluster)
		} else if mcm.Spec.From != "" && hub != mcm.Spec.From {
			notFoundClusters = append(notFoundClusters, cluster)
		}
	}

	// cluster has been migrated
	if len(movedClusters) == len(mcm.Spec.IncludedManagedClusters) {
		mcm.Status.Phase = migrationv1alpha1.PhaseCompleted
		condMessage = fmt.Sprintf("The clusters has been migrated into the hub %s", mcm.Spec.To)
		return false, nil
	}

	// not found validated clusters
	if len(notFoundClusters) > 0 {
		mcm.Status.Phase = migrationv1alpha1.PhaseFailed
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonClusterNotFound
		clusters := notFoundClusters
		if len(notFoundClusters) > 3 {
			clusters = append(clusters[:3], "...")
		}
		condMessage = fmt.Sprintf("The validated clusters %v are not found in the source hub %s", clusters,
			mcm.Spec.From)
		return true, nil
	}

	return false, nil
}

func getClusterWithHub(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string]string, error) {
	db := database.GetGorm()
	managedClusterMap := make(map[string]string)

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name FROM status.managed_clusters
			WHERE cluster_name IN (?)`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to get migrating clusters - %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var leafHubName, managedClusterName string
		if err := rows.Scan(&leafHubName, &managedClusterName); err != nil {
			return nil, fmt.Errorf("failed to scan hub and managed cluster name - %w", err)
		}
		managedClusterMap[managedClusterName] = leafHubName
	}
	return managedClusterMap, nil
}
