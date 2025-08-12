package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	ConditionReasonHubClusterInvalid     = "HubClusterInvalid"
	ConditionReasonClusterNotFound       = "ClusterNotFound"
	ConditionReasonClusterConflict       = "ClusterConflict"
	ConditionReasonClusterValidateFailed = "ClusterValidateFailed"

	ConditionReasonResourceValidated = "ResourceValidated"
	ConditionReasonResourceInvalid   = "ResourceInvalid"

	// Validation constants
	maxClusterMessagesDisplay = 3

	// Managed cluster conditions
	conditionTypeAvailable = "ManagedClusterConditionAvailable"
)

// Only configmap and secret are allowed
var AllowedKinds = map[string]bool{
	"configmap": true,
	"secret":    true,
}

type ManagedClusterInfo struct {
	leafHubName string
	annotations map[string]string
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
	log.Info("migration validating from hub")

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
	log.Info("migration validating to hub")

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

	log.Info("migration validating clusters")

	// verify managedclusters
	if err = m.validateClustersForMigration(mcm, &condition); err != nil {
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

// ValidationResult represents the result of validating a single cluster
type ValidationResult struct {
	ErrorMessage    string
	ConditionReason string
}

// validateSingleCluster validates a single cluster,
// returns validation result with error message and condition reason
func validateSingleCluster(
	cluster string,
	clusterToClusterInfoMap map[string]ManagedClusterInfo,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) ValidationResult {
	clusterInfo, ok := clusterToClusterInfoMap[cluster]
	if !ok {
		return ValidationResult{
			ErrorMessage:    fmt.Sprintf("cluster %s not found in database", cluster),
			ConditionReason: ConditionReasonClusterNotFound,
		}
	}

	// check not hosted using annotations
	if isHostedCluster(clusterInfo) {
		return ValidationResult{
			ErrorMessage:    fmt.Sprintf("cluster %s is hosted", cluster),
			ConditionReason: ConditionReasonResourceInvalid,
		}
	}

	// if cluster in destination hub
	if clusterInfo.leafHubName == mcm.Spec.To {
		return ValidationResult{
			ErrorMessage:    fmt.Sprintf("cluster %s is already on hub %s", cluster, mcm.Spec.To),
			ConditionReason: ConditionReasonClusterConflict,
		}
	}

	// if cluster not in source hub
	if clusterInfo.leafHubName != mcm.Spec.From {
		return ValidationResult{
			ErrorMessage:    fmt.Sprintf("cluster %s not found in hub %s", cluster, mcm.Spec.From),
			ConditionReason: ConditionReasonClusterNotFound,
		}
	}

	return ValidationResult{}
}

// isHostedCluster checks if a managed cluster is hosted
func isHostedCluster(clusterInfo ManagedClusterInfo) bool {
	return clusterInfo.annotations != nil &&
		clusterInfo.annotations[constants.AnnotationClusterDeployMode] == constants.ClusterDeployModeHosted
}

// validateClustersForMigration validates each cluster for migration for each failed cluster.
// Returns a single error if any cluster fails validation.
func (m *ClusterMigrationController) validateClustersForMigration(
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition *metav1.Condition,
) error {
	// Early return if no clusters to validate
	if len(mcm.Spec.IncludedManagedClusters) == 0 {
		condition.Reason = ConditionReasonClusterNotFound
		return fmt.Errorf("no managed clusters specified for migration")
	}

	// get the clusters in database
	clusterToClusterInfoMap, err := getclusterToClusterInfoMap(mcm)
	if err != nil {
		condition.Reason = ConditionReasonClusterNotFound
		return fmt.Errorf("failed to get cluster information: %w", err)
	}

	if len(clusterToClusterInfoMap) == 0 {
		condition.Reason = ConditionReasonClusterNotFound
		return fmt.Errorf("no valid managed clusters found in database: %v", mcm.Spec.IncludedManagedClusters)
	}

	failedClustersCount := 0

	// verify clusters in mcm
	for _, cluster := range mcm.Spec.IncludedManagedClusters {
		log.Infof("verify cluster: %v", cluster)

		if singleResult := validateSingleCluster(cluster, clusterToClusterInfoMap, mcm); singleResult.ErrorMessage != "" {
			failedClustersCount += 1
			// Record validation error as Kubernetes event
			m.EventRecorder.Eventf(mcm, corev1.EventTypeWarning, "ValidationFailed", singleResult.ErrorMessage)
			log.Warnf(singleResult.ErrorMessage)
			// Set condition reason based on validation result - use the last error type encountered
			condition.Reason = singleResult.ConditionReason
		}
	}
	log.Infof("%v clusters verify failed", failedClustersCount)

	if failedClustersCount > 0 {
		return fmt.Errorf("%v clusters validate failed, please check the events for details", failedClustersCount)
	}

	return nil
}

// managedClusterRecord represents a row from the managed_clusters table
// used for efficient database queries via GORM
type managedClusterRecord struct {
	ClusterName string `gorm:"column:cluster_name"`  // The name of the managed cluster
	LeafHubName string `gorm:"column:leaf_hub_name"` // The hub cluster that manages this cluster
	Annotations string `gorm:"column:annotations"`   // JSON annotations from payload->metadata->annotations
}

func (managedClusterRecord) TableName() string {
	return "status.managed_clusters"
}

func getclusterToClusterInfoMap(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string]ManagedClusterInfo, error) {
	db := database.GetGorm()
	if db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	if len(mcm.Spec.IncludedManagedClusters) == 0 {
		return make(map[string]ManagedClusterInfo), nil
	}

	var records []managedClusterRecord
	err := db.Select("cluster_name", "leaf_hub_name", "payload->'metadata'->'annotations' as annotations").
		Where("cluster_name IN (?) AND deleted_at IS NULL", mcm.Spec.IncludedManagedClusters).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("failed to query managed clusters: %w", err)
	}

	managedClusterMap := make(map[string]ManagedClusterInfo, len(records))
	for _, record := range records {
		var annotations map[string]string
		if record.Annotations != "" {
			if err := json.Unmarshal([]byte(record.Annotations), &annotations); err != nil {
				return nil, fmt.Errorf("failed to unmarshal annotations json: %w", err)
			}
		}
		if annotations == nil {
			annotations = make(map[string]string)
		}

		managedClusterMap[record.ClusterName] = ManagedClusterInfo{
			leafHubName: record.LeafHubName,
			annotations: annotations,
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
	if mc.Annotations != nil && mc.Annotations[constants.AnnotationONMulticlusterHub] == "true" {
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
