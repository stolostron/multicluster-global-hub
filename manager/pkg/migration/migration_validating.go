package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

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
	labels      map[string]string
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
	log.Infof("migration: %v validating", mcm.Name)

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeValidated) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseValidating {
		return false, nil
	}
	log.Info("migration validating")
	nextPhase := migrationv1alpha1.PhaseInitializing

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeValidated,
		Status:  metav1.ConditionTrue,
		Reason:  ConditionReasonResourceValidated,
		Message: "Migration resources have been validated",
	}

	var err error
	defer func() {
		if err != nil {
			condition.Message = err.Error()
			condition.Status = metav1.ConditionFalse
			nextPhase = migrationv1alpha1.PhaseFailed
			if m.EventRecorder != nil {
				m.EventRecorder.Eventf(mcm, corev1.EventTypeWarning, "ValidationFailed", condition.Message)
			}
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
		return false, err
	}
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.From); err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		return false, fmt.Errorf("source hub %s: %v", mcm.Spec.From, err)
	}
	log.Info("migration validating to hub")

	// verify toHub
	if mcm.Spec.To == "" {
		err = fmt.Errorf("destination hub is not specified")
		return false, err
	}
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.To); err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		err = fmt.Errorf("destination hub %s: %v", mcm.Spec.To, err)
		return false, err
	}

	log.Info("migration validating clusters")

	// Get migrate clusters
	clusters, err := m.getMigrationClusters(ctx, mcm)
	if err != nil {
		return false, err
	}

	log.Debugf("migrate name:%v, clusters: %v", mcm.Name, clusters)

	// should reconcile when no managedcluster found
	if len(clusters) == 0 {
		condition.Message = "Waiting to validat migration clusters"
		condition.Status = metav1.ConditionFalse
		condition.Reason = ConditionReasonWaiting
		nextPhase = migrationv1alpha1.PhaseValidating
		return true, nil
	}

	SetClusterList(string(mcm.UID), clusters)
	// Store clusters in ConfigMap
	if err := m.storeClustersToConfigMap(ctx, mcm, clusters); err != nil {
		return false, fmt.Errorf("failed to store clusters to ConfigMap: %w", err)
	}
	// verify managedclusters
	if err = m.validateClustersForMigration(mcm, &condition); err != nil {
		return false, err
	}
	return false, nil
}

func (m *ClusterMigrationController) getMigrationClusters(
	ctx context.Context, mcm *migrationv1alpha1.ManagedClusterMigration,
) ([]string, error) {
	// Fallback to spec if available
	if len(mcm.Spec.IncludedManagedClusters) != 0 {
		return mcm.Spec.IncludedManagedClusters, nil
	}

	if mcm.Spec.IncludedManagedClustersPlacementRef == "" {
		return nil, fmt.Errorf("no clusters in migration cr")
	}

	if errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseValidating); errMsg != "" {
		return nil, fmt.Errorf("get IncludedManagedClusters from hub %s with err :%s", mcm.Spec.From, errMsg)
	}

	// get clusters from cache first
	clusters := GetClusterList(string(mcm.GetUID()))

	if len(clusters) > 0 {
		log.Debugf("clusters: %v", clusters)
		return clusters, nil
	}

	// get cluster from configmap first
	clusters, err := m.getClustersFromExistingConfigMap(ctx, mcm.Name, mcm.Namespace)
	if err != nil {
		return nil, err
	}
	if len(clusters) > 0 {
		return clusters, nil
	}

	// send event to source hub and get placement name from bundle
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseValidating) {
		err := m.sendEventToSourceHub(ctx, mcm.Spec.From, mcm, migrationv1alpha1.PhaseValidating,
			nil, nil, "")
		if err != nil {
			return nil, err
		}
		log.Infof("sent validating events to source hubs: %s with placement: %s",
			mcm.Spec.From, mcm.Spec.IncludedManagedClustersPlacementRef)
		log.Infof("sent initialing events to source hubs: %s", mcm.Spec.From)
		SetStarted(string(mcm.GetUID()), mcm.Spec.From, migrationv1alpha1.PhaseValidating)
	}

	// need reconcile to wait clusterlist
	return nil, nil
}

// storeClustersToConfigMap creates or updates a ConfigMap with the clusters data for the migration
// The ConfigMap is named with the migration name and has owner reference to the migration object
func (m *ClusterMigrationController) storeClustersToConfigMap(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, clusters []string,
) error {
	clustersData, err := json.Marshal(clusters)
	if err != nil {
		return fmt.Errorf("failed to marshal clusters data: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migration.Name,
			Namespace: migration.Namespace,
		},
	}
	err = controllerutil.SetOwnerReference(migration, configMap, m.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference for configmap: %s", configMap.Name)
	}
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, m.Client, configMap, func() error {
			configMap.Data = map[string]string{
				"clusters": string(clustersData),
			}
			return nil
		})
		log.Infof("save configmap for migration %s, operation: %v", configMap.Name, operation)
		return err
	})
	return err
}

// getClustersFromExistingConfigMap attempts to retrieve clusters from an existing ConfigMap
// Returns clusters slice, found boolean, and error
func (m *ClusterMigrationController) getClustersFromExistingConfigMap(ctx context.Context,
	configMapName, namespace string,
) ([]string, error) {
	existingConfigMap := &corev1.ConfigMap{}
	err := m.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: namespace,
	}, existingConfigMap)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// ConfigMap exists, get clusters from it
	if clustersJSON, exists := existingConfigMap.Data["clusters"]; exists {
		var clusters []string
		if err := json.Unmarshal([]byte(clustersJSON), &clusters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal clusters from ConfigMap: %w", err)
		}
		log.Infof("retrieved clusters from existing ConfigMap %s/%s for migration %s", namespace, configMapName, configMapName)
		return clusters, nil
	}

	return nil, nil
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

	// check not local cluster
	if clusterInfo.labels != nil && clusterInfo.labels[constants.LocalClusterName] == "true" {
		return ValidationResult{
			ErrorMessage:    fmt.Sprintf("cluster %s is local cluster", cluster),
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
	// get the clusters in database
	clusterToClusterInfoMap, err := m.getclusterToClusterInfoMap(mcm)
	if err != nil {
		condition.Reason = ConditionReasonClusterNotFound
		return fmt.Errorf("failed to get cluster information: %w", err)
	}

	if len(clusterToClusterInfoMap) == 0 {
		condition.Reason = ConditionReasonClusterNotFound
		return fmt.Errorf("no valid managed clusters found in database")
	}

	failedClustersCount := 0
	clusters := GetClusterList(string(mcm.UID))
	// verify clusters in mcm
	for _, cluster := range clusters {
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
	Labels      string `gorm:"column:labels"`        // JSON labels from payload->metadata->labels
}

func (managedClusterRecord) TableName() string {
	return "status.managed_clusters"
}

func (m *ClusterMigrationController) getclusterToClusterInfoMap(
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (map[string]ManagedClusterInfo, error) {
	db := database.GetGorm()
	if db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	clusters := GetClusterList(string(mcm.UID))

	var records []managedClusterRecord
	err := db.Select(
		"cluster_name", "leaf_hub_name",
		"payload->'metadata'->'annotations' as annotations", "payload->'metadata'->'labels' as labels").
		Where("cluster_name IN (?) AND deleted_at IS NULL", clusters).
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

		var labels map[string]string
		if record.Labels != "" {
			if err := json.Unmarshal([]byte(record.Labels), &labels); err != nil {
				return nil, fmt.Errorf("failed to unmarshal labels json: %w", err)
			}
		}

		managedClusterMap[record.ClusterName] = ManagedClusterInfo{
			leafHubName: record.LeafHubName,
			annotations: annotations,
			labels:      labels,
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
