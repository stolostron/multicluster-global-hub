package placement

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

// NewManagerPlacementDecisionsBundle creates a new instance of PlacementDecisionsBundle.
func NewManagerPlacementDecisionsBundle() bundle.ManagerBundle {
	return &PlacementDecisionsBundle{}
}

// PlacementDecisionsBundle abstracts management of placement-decisions bundle.
type PlacementDecisionsBundle struct {
	base.BaseManagerBundle
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
