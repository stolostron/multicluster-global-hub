package bundle

import (
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// NewManagedClustersStatusBundle creates a new instance of ManagedClustersStatusBundle.
func NewManagedClustersStatusBundle() Bundle {
	return &ManagedClustersStatusBundle{}
}

// ManagedClustersStatusBundle abstracts management of managed clusters bundle.
type ManagedClustersStatusBundle struct {
	baseBundle
	Objects []*clusterv1.ManagedCluster `json:"objects"`
}

// GetObjects returns the objects in the bundle.
func (bundle *ManagedClustersStatusBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
