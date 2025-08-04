package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ConditionReasonHubClusterInvalid = "HubClusterInvalid"
	ConditionReasonClusterNotFound   = "ClusterNotFound"
	ConditionReasonClusterConflict   = "ClusterConflict"
	ConditionReasonResourceValidated = "ResourceValidated"
	ConditionReasonResourceInvalid   = "ResourceInvalid"

	// Validation constants
	maxClusterMessagesDisplay = 3
	validationFailureFormat   = "validation failed for clusters: %v"
	clusterNotFoundMsg        = "cluster %s not found in database"
	clusterNotAvailableMsg    = "cluster %s is not available"
	clusterIsHostedMsg        = "cluster %s is hosted"
	clusterAlreadyOnHubMsg    = "cluster %s is already on hub %s"
	clusterNotFoundInHubMsg   = "cluster %s not found in hub %s"

	// SQL table and column names
	tableManagedClusters = "status.managed_clusters"
	columnLeafHubName    = "leaf_hub_name"
	columnClusterName    = "cluster_name"
	columnPayload        = "payload"
	columnDeletedAt      = "deleted_at"

	// Managed cluster conditions
	conditionTypeAvailable = "ManagedClusterConditionAvailable"
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

// validating performs comprehensive validation of a ManagedClusterMigration request.
// It validates:
// 1. Source and destination hub clusters (existence, availability, and hub capability)
// 2. Managed clusters (existence, availability, and proper hub assignment)
//
// Validation logic:
// - If clusters do not exist in both from and to hubs, report validation error and mark as failed
// - If clusters exist in source hub and are valid, mark as validated and change phase to initializing
// - If clusters exist in destination hub, mark as failed with 'ClusterConflict' message
//
// Returns:
// - bool: true if validation should continue, false if already processed or deleted
// - error: any error encountered during validation
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
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.From); err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		err = fmt.Errorf("source hub %s: %v", mcm.Spec.From, err)
		return false, nil
	}

	// verify toHub
	if mcm.Spec.To == "" {
		err = fmt.Errorf("destination hub is not specified")
		return false, nil
	}
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.To); err != nil {
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
	if err = validateClustersForMigration(ctx, m.Client, mcm, condition); err != nil {
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
	if len(clusters) <= maxClusterMessagesDisplay {
		return clusters
	}
	return append(clusters[:maxClusterMessagesDisplay], "...")
}

// validateSingleCluster validates a single cluster and returns an error message if validation fails
func validateSingleCluster(cluster string, clusterToClusterInfoMap map[string]ManagedClusterInfo, mcm *migrationv1alpha1.ManagedClusterMigration) string {
	clusterInfo, ok := clusterToClusterInfoMap[cluster]
	if !ok {
		return fmt.Sprintf(clusterNotFoundMsg, cluster)
	}

	mc := &clusterInfo.managedCluster

	// check available
	if !isManagedClusterAvailable(mc) {
		return fmt.Sprintf(clusterNotAvailableMsg, cluster)
	}

	// check not hosted
	if isHostedCluster(mc) {
		return fmt.Sprintf(clusterIsHostedMsg, cluster)
	}

	// if cluster in destination hub
	if clusterInfo.leafHubName == mcm.Spec.To {
		return fmt.Sprintf(clusterAlreadyOnHubMsg, cluster, mcm.Spec.To)
	}

	// if cluster not in source hub
	if clusterInfo.leafHubName != mcm.Spec.From {
		return fmt.Sprintf(clusterNotFoundInHubMsg, cluster, mcm.Spec.From)
	}

	return ""
}

// isHostedCluster checks if a managed cluster is hosted
func isHostedCluster(mc *clusterv1.ManagedCluster) bool {
	return mc.Annotations != nil && mc.Annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted
}

// validateClustersForMigration validates each cluster for migration for each failed cluster.
// Returns a single error if any cluster fails validation.
func validateClustersForMigration(ctx context.Context, c client.Client, mcm *migrationv1alpha1.ManagedClusterMigration, condition metav1.Condition) error {
	// Early return if no clusters to validate
	if len(mcm.Spec.IncludedManagedClusters) == 0 {
		return fmt.Errorf("no managed clusters specified for migration")
	}

	// get the clusters in database
	clusterToClusterInfoMap, err := getclusterToClusterInfoMap(mcm)
	if err != nil {
		return fmt.Errorf("failed to get cluster information: %w", err)
	}

	if len(clusterToClusterInfoMap) == 0 {
		return fmt.Errorf("no valid managed clusters found in database: %v", mcm.Spec.IncludedManagedClusters)
	}

	// Pre-allocate slice with known maximum capacity
	failedClusters := make([]string, 0, len(mcm.Spec.IncludedManagedClusters))

	// verify clusters in mcm
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		if errorMsg := validateSingleCluster(cluster, clusterToClusterInfoMap, mcm); errorMsg != "" {
			log.Error(errorMsg)
			failedClusters = append(failedClusters, errorMsg)
		}
	}

	if len(failedClusters) > 0 {
		return fmt.Errorf(validationFailureFormat, messageClusters(failedClusters))
	}
	return nil
}

// managedClusterRecord represents a row from the managed_clusters table
// used for efficient database queries via GORM
type managedClusterRecord struct {
	LeafHubName string `gorm:"column:leaf_hub_name"` // The hub cluster that manages this cluster
	ClusterName string `gorm:"column:cluster_name"`  // Name of the managed cluster
	Payload     string `gorm:"column:payload"`       // JSON payload containing ManagedCluster object
}

// TableName specifies the database table name for GORM ORM mapping
func (managedClusterRecord) TableName() string {
	return tableManagedClusters
}

// getclusterToClusterInfoMap retrieves cluster information from the database for all clusters
// specified in the migration request. It uses GORM to efficiently query the managed_clusters table
// and returns a map of cluster names to their detailed information including hub assignment.
//
// This function optimizes database access by:
// - Using a single query with IN IncludedManagedClusters for multiple clusters
// - Pre-allocating map capacity based on expected results
func getclusterToClusterInfoMap(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string]ManagedClusterInfo, error) {
	db := database.GetGorm()
	if db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	if len(mcm.Spec.IncludedManagedClusters) == 0 {
		return make(map[string]ManagedClusterInfo), nil
	}

	var records []managedClusterRecord
	err := db.Select(columnLeafHubName, columnClusterName, columnPayload).
		Where(columnClusterName+" IN (?) AND "+columnDeletedAt+" IS NULL", mcm.Spec.IncludedManagedClusters).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query managed clusters: %w", err)
	}

	managedClusterMap := make(map[string]ManagedClusterInfo, len(records))
	for _, record := range records {
		var mc clusterv1.ManagedCluster
		if err := json.Unmarshal([]byte(record.Payload), &mc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal managed cluster json: %w", err)
		}
		managedClusterMap[record.ClusterName] = ManagedClusterInfo{
			leafHubName:    record.LeafHubName,
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
		if cond.Type == conditionTypeAvailable && cond.Status == "True" {
			return true
		}
	}
	return false
}

// isHubCluster determines if ManagedCluster is a hub cluster
func isHubCluster(ctx context.Context, c client.Client, mc *clusterv1.ManagedCluster) bool {
	// Has annotation addon.open-cluster-management.io/on-multicluster-hub=true
	if mc.Annotations != nil && mc.Annotations[operatorconstants.AnnotationONMulticlusterHub] == "true" {
		return true
	}
	// local-cluster and has deployment multicluster-global-hub-agent
	if mc.Labels != nil && mc.Labels[constants.LocalClusterName] == "true" {
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
