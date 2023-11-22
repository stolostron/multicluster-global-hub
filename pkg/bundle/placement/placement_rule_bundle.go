package placement

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
)

var _ bundle.ManagerBundle = (*PlacementRulesBundle)(nil)

// NewManagerPlacementRulesBundle creates a new instance of PlacementRulesBundle.
func NewManagerPlacementRulesBundle() bundle.ManagerBundle {
	return &PlacementRulesBundle{}
}

// PlacementRulesBundle abstracts management of placement-rules bundle.
type PlacementRulesBundle struct {
	base.BaseManagerBundle
	Objects []*placementrulev1.PlacementRule `json:"objects"`
}

// GetObjects return all the objects that the bundle holds.
func (bundle *PlacementRulesBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
