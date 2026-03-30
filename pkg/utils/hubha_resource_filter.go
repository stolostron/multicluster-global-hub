package utils

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// HubHAResourceFilter defines which resources should be synced for Hub HA
type HubHAResourceFilter struct {
	// Required label keys for secrets and configmaps
	requiredSecretConfigMapLabels []string
}

// NewHubHAResourceFilter creates a new resource filter for Hub HA
func NewHubHAResourceFilter() *HubHAResourceFilter {
	return &HubHAResourceFilter{
		requiredSecretConfigMapLabels: []string{
			"cluster.open-cluster-management.io/type",
			"hive.openshift.io/secret-type",
			"cluster.open-cluster-management.io/backup",
		},
	}
}

// ShouldSyncResource determines if a resource should be synced for Hub HA
// This is called per-object to filter individual resource instances
func (f *HubHAResourceFilter) ShouldSyncResource(obj client.Object, gvk schema.GroupVersionKind) bool {
	// Exclude resources explicitly marked to be excluded from backup
	labels := obj.GetLabels()
	if labels != nil && labels["velero.io/exclude-from-backup"] == "true" {
		return false
	}

	kind := gvk.Kind

	// Special handling for Secrets and ConfigMaps - only sync those with required labels
	if kind == "Secret" || kind == "ConfigMap" {
		return f.shouldSyncSecretOrConfigMap(obj)
	}

	// All other resources in the hardcoded list should be synced
	return true
}

// shouldSyncSecretOrConfigMap checks if a Secret or ConfigMap should be synced
// based on required labels
func (f *HubHAResourceFilter) shouldSyncSecretOrConfigMap(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}

	// Check if it has any of the required labels
	for _, requiredLabel := range f.requiredSecretConfigMapLabels {
		if _, exists := labels[requiredLabel]; exists {
			return true
		}
	}

	return false
}

// IsActiveHub checks if the managed cluster is an active ACM hub
func IsActiveHub(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[constants.GHHubRoleLabelKey] == constants.GHHubRoleActive
}

// IsStandbyHub checks if the managed cluster is a standby ACM hub
func IsStandbyHub(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[constants.GHHubRoleLabelKey] == constants.GHHubRoleStandby
}
