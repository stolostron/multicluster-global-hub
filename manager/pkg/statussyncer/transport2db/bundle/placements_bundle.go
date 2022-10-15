package bundle

import (
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewPlacementsBundle creates a new instance of PlacementsBundle.
func NewPlacementsBundle() status.Bundle {
	return &PlacementsBundle{}
}

// PlacementsBundle abstracts management of placement bundle.
type PlacementsBundle struct {
	baseBundle
	Objects []*clusterv1beta1.Placement `json:"objects"`
}

// GetObjects return all the objects that the bundle holds.
func (bundle *PlacementsBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
