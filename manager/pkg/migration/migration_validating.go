package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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
		}

		if condition.Reason == ConditionReasonWaiting &&
			time.Since(getLastTransitionTime(mcm)) > getTimeout(migrationv1alpha1.PhaseValidating) {
			condition.Reason = ConditionReasonTimeout
			condition.Message = fmt.Sprintf("Timeout: %s", condition.Message)
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
		return false, err
	}
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.From); err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		return false, err
	}
	log.Info("migration validating to hub")

	// verify toHub
	if mcm.Spec.To == "" {
		err = fmt.Errorf("target hub is not specified")
		return false, err
	}
	if err = validateHubCluster(ctx, m.Client, mcm.Spec.To); err != nil {
		condition.Reason = ConditionReasonHubClusterInvalid
		return false, err
	}

	log.Info("migration validating clusters")

	// Get migrate clusters
	requeue, err := m.validateMigrationClusters(ctx, mcm)
	if err != nil {
		return false, err
	}
	if requeue {
		condition.Message = "Waiting to validate migration clusters"
		condition.Status = metav1.ConditionFalse
		condition.Reason = ConditionReasonWaiting
		nextPhase = migrationv1alpha1.PhaseValidating
		return true, nil
	}

	log.Debugf("migrate name:%v, clusters: %v", mcm.Name, GetClusterList(string(mcm.UID)))

	// Store clusters in ConfigMap
	if err := m.storeClustersToConfigMap(ctx, mcm, GetClusterList(string(mcm.UID))); err != nil {
		return false, fmt.Errorf("failed to store clusters to ConfigMap: %w", err)
	}
	return false, nil
}

func (m *ClusterMigrationController) validateMigrationClusters(
	ctx context.Context, mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	// validate clusters from source hub
	requeue, err := m.validateClustersInHub(ctx, mcm, mcm.Spec.From)
	if err != nil {
		return false, err
	}
	if requeue {
		return true, nil
	}

	// validate clusters from target hub
	requeue, err = m.validateClustersInHub(ctx, mcm, mcm.Spec.To)
	if err != nil {
		return false, err
	}
	if requeue {
		return true, nil
	}

	return false, nil
}

// validateClustersInHub sends validation events to source hub and processes the results
func (m *ClusterMigrationController) validateClustersInHub(
	ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	hub string,
) (bool, error) {
	// send event to source/to hub validate the clusters
	if !GetStarted(string(mcm.GetUID()), hub, migrationv1alpha1.PhaseValidating) {
		var clusterList []string
		if len(mcm.Spec.IncludedManagedClusters) != 0 {
			clusterList = mcm.Spec.IncludedManagedClusters
		}
		err := m.sendEventToSourceHub(ctx, hub, mcm, migrationv1alpha1.PhaseValidating,
			clusterList, nil, "")
		if err != nil {
			return false, err
		}
		log.Infof("sent validating events to source hubs: %s", hub)
		SetStarted(string(mcm.GetUID()), hub, migrationv1alpha1.PhaseValidating)
	}

	if errMsg := GetErrorMessage(string(mcm.GetUID()), hub, migrationv1alpha1.PhaseValidating); errMsg != "" {
		// send cluster error to events
		m.handleErrorList(mcm, hub, migrationv1alpha1.PhaseValidating)
		return false, fmt.Errorf("failed to validate clusters in hub %s, %s", hub, errMsg)
	}

	// Wait validate in source hub
	if !GetFinished(string(mcm.GetUID()), hub, migrationv1alpha1.PhaseValidating) {
		return true, nil
	}

	return false, nil
}

func (m *ClusterMigrationController) handleErrorList(
	mcm *migrationv1alpha1.ManagedClusterMigration, hub, phase string,
) {
	errList := GetClusterErrors(string(mcm.GetUID()), hub, phase)
	if errList == nil {
		return
	}
	for _, errMsg := range errList {
		m.EventRecorder.Eventf(mcm, corev1.EventTypeWarning, "ValidationFailed", errMsg)
	}
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

// validateHubCluster validates if ManagedCluster is a hub cluster and is ready, returns error if not valid
func validateHubCluster(ctx context.Context, c client.Client, name string) error {
	mc := &clusterv1.ManagedCluster{}
	if err := c.Get(ctx, types.NamespacedName{Name: name}, mc); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("managed hub cluster %s is not found", name)
		}
		return err
	}
	// Check cluster Available
	if !isManagedClusterAvailable(mc) {
		return fmt.Errorf("managed hub cluster %s is not ready", name)
	}

	// Determine if it is a hub cluster
	if !isHubCluster(ctx, c, mc) {
		return fmt.Errorf("%s is not a managed hub cluster", name)
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
