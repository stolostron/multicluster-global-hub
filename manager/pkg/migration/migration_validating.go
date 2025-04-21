package migration

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	ConditionReasonHubClusterNotFound = "HubClusterNotFound"
	ConditionReasonClusterNotFound    = "ClusterNotFound"
	ConditionReasonClusterConflict    = "ClusterConflict"
	ConditionReasonResourceValidated  = "ResourceValidated"
)

// Validation:
// 1. Verify if the migrating clusters exist in the current hubs
//   - If the clusters do not exist in both the from and to hubs, report a validating error and mark status as failed
//   - If the clusters exist in the source hub, mark the status as validated and change the phase to initializing.
//   - If the clusters exist in the destination hub, mark the status as failed, raise the 'ClusterConflict' message.
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

	// Skip validation if the ResourceValidated is true,
	// or if the phase status is not 'validating' (indicating it's currently in a different stage)
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

	// Verify if both source and destination hubs exist
	db := database.GetGorm()

	var fromHubErr, toHubErr error
	if mcm.Spec.From != "" {
		fromHubErr = db.Model(&models.LeafHub{LeafHubName: mcm.Spec.From}).First(&models.LeafHub{}).Error
	}
	toHubErr = db.Model(&models.LeafHub{LeafHubName: mcm.Spec.To}).First(&models.LeafHub{}).Error

	if errors.Is(fromHubErr, gorm.ErrRecordNotFound) || errors.Is(toHubErr, gorm.ErrRecordNotFound) {
		mcm.Status.Phase = migrationv1alpha1.PhaseFailed
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonHubClusterNotFound
		switch {
		case errors.Is(fromHubErr, gorm.ErrRecordNotFound):
			condMessage = fmt.Sprintf("Not found the source hub: %s", mcm.Spec.From)
		case errors.Is(toHubErr, gorm.ErrRecordNotFound):
			condMessage = fmt.Sprintf("Not found the destination hub: %s", mcm.Spec.To)
		}
		return false, nil
	}
	if fromHubErr != nil {
		err = fromHubErr
	} else {
		err = toHubErr
	}
	if err != nil {
		condReason = ConditionReasonHubClusterNotFound
		return false, err
	}

	// verify the clusters: hub in database
	clusterWithHub, err := getClusterWithHub(mcm)

	notFoundClusters := []string{}
	clustersInDestinationHub := []string{}
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		hub := clusterWithHub[cluster]
		if hub == mcm.Spec.To {
			clustersInDestinationHub = append(clustersInDestinationHub, cluster)
		} else if mcm.Spec.From != "" && hub != mcm.Spec.From {
			notFoundClusters = append(notFoundClusters, cluster)
		}
	}

	// clusters have been in the destination hub
	if len(clustersInDestinationHub) > 0 {
		mcm.Status.Phase = migrationv1alpha1.PhaseFailed
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonClusterConflict
		condMessage = fmt.Sprintf("The clusters %v have been in the hub cluster %s",
			messageClusters(clustersInDestinationHub), mcm.Spec.To)
		return false, nil
	}

	// not found validated clusters
	if len(notFoundClusters) > 0 {
		mcm.Status.Phase = migrationv1alpha1.PhaseFailed
		condStatus = metav1.ConditionFalse
		condReason = ConditionReasonClusterNotFound
		condMessage = fmt.Sprintf("The validated clusters %v are not found in the source hub %s",
			messageClusters(notFoundClusters), mcm.Spec.From)
		return true, nil
	}

	return false, nil
}

// messageClusters is used to prevent too many messages from appearing in the status.
// It truncates the list to show only the first three clusters.
func messageClusters(clusters []string) []string {
	messages := clusters
	if len(clusters) > 3 {
		messages = append(clusters[:3], "...")
	}
	return messages
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
