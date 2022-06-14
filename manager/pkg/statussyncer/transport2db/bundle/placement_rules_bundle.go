package bundle

import placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"

// NewPlacementRulesBundle creates a new instance of PlacementRulesBundle.
func NewPlacementRulesBundle() Bundle {
	return &PlacementRulesBundle{}
}

// PlacementRulesBundle abstracts management of placement-rules bundle.
type PlacementRulesBundle struct {
	baseBundle
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
