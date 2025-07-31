package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ConditionReasonHubClusterInvalid = "HubClusterInvalid"
	ConditionReasonClusterNotFound   = "ClusterNotFound"
	ConditionReasonClusterConflict   = "ClusterConflict"
	ConditionReasonResourceValidated = "ResourceValidated"
	ConditionReasonResourceInvalid   = "ResourceInvalid"
)

// Only configmap and secret are allowed
var AllowedKinds = map[string]bool{
	"configmap": true,
	"secret":    true,
}

type ManagedClusterInfo struct {
	leafHubName    string
	managedCluster clusterv1.ManagedCluster
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
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeValidated) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseValidating {
		return false, nil
	}
	log.Info("migration validating")

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeValidated,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionReasonResourceValidated,
		Message: "Migration resources have been validated",
	}

	var err error
	defer func() {
		nextPhase := migrationv1alpha1.PhaseInitializing
		if err != nil {
			condition.Message = err.Error()
			condition.Status = metav1.ConditionFalse
			nextPhase = migrationv1alpha1.PhaseFailed
		}
		err = m.UpdateStatusWithRetry(ctx, mcm, condition, nextPhase)
		if err != nil {
			log.Errorf("failed to update the %s condition: %v", condition.Type, err)
		}
	}()

	// verify fromHub
	if mcm.Spec.From == "" {
		err = fmt.Errorf("source hub is not specified")
		return false, nil
	}
	err = validateHubCluster(ctx, m.Client, mcm.Spec.From)
	if err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		err = fmt.Errorf("source hub %s: %v", mcm.Spec.From, err)
		return false, nil
	}

	// verify toHub
	if mcm.Spec.To == "" {
		err = fmt.Errorf("destination hub is not specified")
		return false, nil
	}
	err = validateHubCluster(ctx, m.Client, mcm.Spec.To)
	if err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		err = fmt.Errorf("destination hub %s: %v", mcm.Spec.To, err)
		return false, nil
	}

	// verify toHub
	if mcm.Spec.To == "" {
		err = fmt.Errorf("destination hub is not specified")
		return false, nil
	}
	err = validateHubCluster(ctx, m.Client, mcm.Spec.To)
	if err != nil {
		return false, err
	}

	// verify managedclusters
	err = validateClustersForMigration(ctx, m.Client, mcm, condition)
	if err != nil {
		return false, nil
	}
	return true, nil
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

// validateClustersForMigration validates each cluster for migration for each failed cluster.
// Returns a single error if any cluster fails validation.
func validateClustersForMigration(ctx context.Context, c client.Client, mcm *migrationv1alpha1.ManagedClusterMigration, condition metav1.Condition) error {
	var failedClusters []string
	// get the clusters in database
	clusterToClusterInfoMap, err := getclusterToClusterInfoMap(mcm)
	if err != nil {
		return err
	}

	if len(clusterToClusterInfoMap) == 0 {
		return fmt.Errorf("invalid managed clusters: %v", mcm.Spec.IncludedManagedClusters)
	}

	// verify clusters in mcm
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		clusterInfo, ok := clusterToClusterInfoMap[cluster]
		if !ok {
			log.Errorf("cluster %s not found in database", cluster)
			failedClusters = append(failedClusters, fmt.Sprintf("cluster %s not found in database", cluster))
			continue
		}
		mc := &clusterInfo.managedCluster
		// check available
		if !isManagedClusterAvailable(mc) {
			log.Errorf("cluster %s is not available", cluster)
			failedClusters = append(failedClusters, fmt.Sprintf("cluster %s is not available", cluster))
			continue
		}

		// check not hosted
		if mc.Annotations != nil && mc.Annotations["import.open-cluster-management.io/klusterlet-deploy-mode"] == "Hosted" {
			log.Errorf("cluster %s is hosted", cluster)
			failedClusters = append(failedClusters, fmt.Sprintf("cluster %s is hosted", cluster))
			continue
		}
		// if cluster in hub2
		if clusterInfo.leafHubName == mcm.Spec.To {
			log.Errorf("cluster %s is already on hub %s", cluster, mcm.Spec.To)
			failedClusters = append(failedClusters, fmt.Sprintf("cluster %s is already on hub %s", cluster, mcm.Spec.To))
			continue
		}
		// if cluster not in hub1
		if clusterInfo.leafHubName != mcm.Spec.From {
			log.Errorf("cluster %s not found in hub %s", cluster, mcm.Spec.From)
			failedClusters = append(failedClusters, fmt.Sprintf("cluster %s not found in hub %s", cluster, mcm.Spec.From))
			continue
		}
	}
	if len(failedClusters) > 0 {
		return fmt.Errorf("validation failed for clusters: %v", failedClusters)
	}
	return nil
}

func getclusterToClusterInfoMap(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string]ManagedClusterInfo, error) {
	db := database.GetGorm()
	if db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}
	managedClusterMap := make(map[string]ManagedClusterInfo)

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name, payload FROM status.managed_clusters
			WHERE cluster_name IN (?) AND deleted_at is NULL`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var leafHubName, managedClusterName, managedClusterJSON string
		if err := rows.Scan(&leafHubName, &managedClusterName, &managedClusterJSON); err != nil {
			return nil, fmt.Errorf("failed to scan hub and managed cluster - %w", err)
		}
		var mc clusterv1.ManagedCluster
		if err := json.Unmarshal([]byte(managedClusterJSON), &mc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal managed cluster json - %w", err)
		}
		managedClusterMap[managedClusterName] = ManagedClusterInfo{
			leafHubName:    leafHubName,
			managedCluster: mc,
		}
	}
	return managedClusterMap, nil
}

// validateHubCluster validates if ManagedCluster is a hub cluster and is ready, returns error if not valid
func validateHubCluster(ctx context.Context, c client.Client, name string) error {
	mc := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, types.NamespacedName{Name: name}, mc); err != nil {
		return err
	}
	// Check cluster Available
	if !isManagedClusterAvailable(mc) {
		return fmt.Errorf("cluster %s is not ready", name)
	}

	// Determine if it is a hub cluster
	if !isHubCluster(ctx, c, mc) {
		return fmt.Errorf("cluster %s is not a hub cluster", name)
	}
	return nil
}

// isManagedClusterAvailable returns true if the ManagedCluster is available (Ready condition is True)
func isManagedClusterAvailable(mc *clusterv1.ManagedCluster) bool {
	for _, cond := range mc.Status.Conditions {
		if cond.Type == "ManagedClusterConditionAvailable" && cond.Status == "True" {
			return true
		}
	}
	return false
}

// isHubCluster determines if ManagedCluster is a hub cluster
func isHubCluster(ctx context.Context, c client.Client, mc *clusterv1.ManagedCluster) bool {
	// Has annotation addon.open-cluster-management.io/on-multicluster-hub=true
	if mc.Annotations != nil && mc.Annotations["addon.open-cluster-management.io/on-multicluster-hub"] == "true" {
		return true
	}
	// local-cluster and has deployment multicluster-global-hub-agent
	if mc.Labels != nil && mc.Labels["local-cluster"] == "true" {
		// Check agent deployment exists for local-cluster
		agentDeploy := &appsv1.Deployment{}
		err := c.Get(ctx, types.NamespacedName{
			Name:      "multicluster-global-hub-agent",
			Namespace: utils.GetDefaultNamespace(),
		}, agentDeploy)
		if err == nil {
			return true
		}
	}
	return false
}
