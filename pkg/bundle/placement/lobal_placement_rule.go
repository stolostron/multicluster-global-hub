package placement

import (
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
)

// LocalPlacementRulesBundle abstracts management of local placement rules bundle.
type LocalPlacementRulesBundle struct {
	base.BaseManagerBundle
	Objects []*placementrulev1.PlacementRule `json:"objects"`
}

// NewManagerLocalPlacementRulesBundle creates a new instance of LocalPlacementRulesBundle.
func NewManagerLocalPlacementRulesBundle() bundle.ManagerBundle {
	return &LocalPlacementRulesBundle{}
}

// GetObjects returns the objects in the bundle.
func (bundle *LocalPlacementRulesBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
