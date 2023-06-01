package bundle

import (
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewPlacementDecisionsBundle creates a new instance of PlacementDecisionsBundle.
func NewPlacementDecisionsBundle() status.Bundle {
	return &PlacementDecisionsBundle{}
}

// PlacementDecisionsBundle abstracts management of placement-decisions bundle.
type PlacementDecisionsBundle struct {
	baseBundle
	Objects []*clusterv1beta1.PlacementDecision `json:"objects"`
}

// GetObjects return all the objects that the bundle holds.
func (bundle *PlacementDecisionsBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
