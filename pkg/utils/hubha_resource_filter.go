package utils

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// HubHAResourceFilter defines which resources should be synced for Hub HA
type HubHAResourceFilter struct {
	// Required label keys for secrets and configmaps
	requiredSecretConfigMapLabels []string
	// localClusterName is the ManagedCluster name for this hub's local cluster.
	localClusterName string
	mu               sync.RWMutex
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

// SetLocalClusterName configures the local cluster ManagedCluster name used to
// exclude hub-local inventory from Hub HA pre-stage sync.
func (f *HubHAResourceFilter) SetLocalClusterName(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.localClusterName = name
}

// ShouldSyncResource determines if a resource should be synced for Hub HA
// This is called per-object to filter individual resource instances
func (f *HubHAResourceFilter) ShouldSyncResource(obj client.Object, gvk schema.GroupVersionKind) bool {
	// Exclude resources explicitly marked to be excluded from backup
	labels := obj.GetLabels()
	if labels != nil && labels["velero.io/exclude-from-backup"] == "true" {
		return false
	}

	if f.shouldExcludeLocalClusterResource(obj, gvk) {
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

func (f *HubHAResourceFilter) shouldExcludeLocalClusterResource(
	obj client.Object, gvk schema.GroupVersionKind,
) bool {
	if gvk.Group == clusterv1.GroupName && gvk.Kind == "ManagedCluster" {
		return IsLocalManagedCluster(obj)
	}
	f.mu.RLock()
	localClusterName := f.localClusterName
	f.mu.RUnlock()
	if localClusterName != "" && obj.GetNamespace() == localClusterName {
		return true
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
