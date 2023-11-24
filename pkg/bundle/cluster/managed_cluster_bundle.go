package cluster

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

var _ bundle.ManagerBundle = (*ManagedClusterBundle)(nil)

// ManagedClusterBundle abstracts management of managed clusters bundle.
type ManagedClusterBundle struct {
	Objects []*clusterv1.ManagedCluster `json:"objects"`
	base.BaseManagerBundle
}

// NewManagerManagedClusterBundle creates a new instance of ManagedClustersStatusBundle.
func NewManagerManagedClusterBundle() bundle.ManagerBundle {
	return &ManagedClusterBundle{}
}

// GetObjects returns the objects in the bundle.
func (bundle *ManagedClusterBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
