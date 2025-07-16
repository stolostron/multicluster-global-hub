package migration

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

const (
	ConditionReasonHubClusterNotFound = "HubClusterNotFound"
	ConditionReasonClusterNotFound    = "ClusterNotFound"
	ConditionReasonClusterConflict    = "ClusterConflict"
	ConditionReasonResourceValidated  = "ResourceValidated"
	ConditionReasonResourceInvalid    = "ResourceInvalid"
)

// Only configmap and secret are allowed
var AllowedKinds = map[string]bool{
	"configmap": true,
	"secret":    true,
}

// DNS Subdomain (RFC 1123) — for ConfigMap, Secret, Namespace, etc.
var dns1123SubdomainRegex = regexp.MustCompile(`^[a-z0-9]([-a-z0-9\.]*[a-z0-9])?$`)

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

	if mcm.Status.Phase == "" || mcm.Status.Phase == migrationv1alpha1.PhasePending {
		mcm.Status.Phase = migrationv1alpha1.PhaseValidating
		if err := m.Client.Status().Update(ctx, mcm); err != nil {
			return false, err
		}
		// initializing the migration status for the instance
		AddMigrationStatus(string(mcm.GetUID()))
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
			condReason = ConditionReasonResourceInvalid
		}
		log.Debugf("validating condition %s(%s): %s", condType, condReason, condMessage)
		e := m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage, migrationStageTimeout)
		if e != nil {
			log.Errorf("failed to update the %s condition: %v", condType, e)
		}
	}()

	// verify if both source and destination hubs exist
	var fromHubErr, toHubErr error
	if mcm.Spec.From != "" {
		fromHubErr = m.Client.Get(ctx, types.NamespacedName{Name: mcm.Spec.From}, &clusterv1.ManagedCluster{})
	}
	toHubErr = m.Client.Get(ctx, types.NamespacedName{Name: mcm.Spec.To}, &clusterv1.ManagedCluster{})

	if errors.IsNotFound(fromHubErr) || errors.IsNotFound(toHubErr) {
		condReason = ConditionReasonHubClusterNotFound
		switch {
		case errors.IsNotFound(fromHubErr):
			err = fmt.Errorf("not found the source hub: %s", mcm.Spec.From)
		case errors.IsNotFound(toHubErr):
			err = fmt.Errorf("not found the destination hub: %s", mcm.Spec.To)
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
		return false, nil
	}

	// verify the clusters in database
	clusterWithHub, err := getClusterWithHub(mcm)
	if err != nil {
		return false, err
	}
	if len(clusterWithHub) == 0 {
		condReason = ConditionReasonClusterNotFound
		err = fmt.Errorf("invalid managed clusters: %v", mcm.Spec.IncludedManagedClusters)
		return false, nil
	}

	notFoundClusters := []string{}
	clustersInDestinationHub := []string{}
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		hub, ok := clusterWithHub[cluster]
		if !ok {
			condReason = ConditionReasonClusterNotFound
			err = fmt.Errorf("not found managed clusters: %s", cluster)
			return false, nil
		}
		if hub == mcm.Spec.To {
			clustersInDestinationHub = append(clustersInDestinationHub, cluster)
		} else if mcm.Spec.From != "" && hub != mcm.Spec.From {
			notFoundClusters = append(notFoundClusters, cluster)
		}
	}

	// clusters have been in the destination hub
	if len(clustersInDestinationHub) > 0 {
		condReason = ConditionReasonClusterConflict
		err = fmt.Errorf("the clusters %v have been in the hub cluster %s",
			messageClusters(clustersInDestinationHub), mcm.Spec.To)
		return false, nil
	}

	// not found validated clusters
	if len(notFoundClusters) > 0 {
		condReason = ConditionReasonClusterNotFound
		err = fmt.Errorf("the validated clusters %v are not found in the source hub %s",
			messageClusters(notFoundClusters), mcm.Spec.From)
		return false, nil
	}

	return false, nil
}

// IsValidResource checks format kind/namespace/name
func IsValidResource(resource string) error {
	parts := strings.Split(resource, "/")
	if len(parts) != 3 {
		return fmt.Errorf("invalid format (must be kind/namespace/name): %s", resource)
	}

	kind, ns, name := strings.ToLower(parts[0]), parts[1], parts[2]

	if !AllowedKinds[kind] {
		return fmt.Errorf("unsupported kind: %s", kind)
	}
	if !dns1123SubdomainRegex.MatchString(ns) {
		return fmt.Errorf("invalid namespace: %s", ns)
	}
	if !dns1123SubdomainRegex.MatchString(name) {
		return fmt.Errorf("invalid name: %s", name)
	}
	return nil
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
	if db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}
	managedClusterMap := make(map[string]string)

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name FROM status.managed_clusters
			WHERE cluster_name IN (?) AND deleted_at is NULL`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, err
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
