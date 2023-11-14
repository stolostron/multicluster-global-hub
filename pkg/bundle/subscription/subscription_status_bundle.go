package subscription

import (
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
)

// NewManagerSubscriptionStatusesBundle creates a new instance of SubscriptionStatusesBundle.
func NewManagerSubscriptionStatusesBundle() bundle.ManagerBundle {
	return &SubscriptionStatusesBundle{}
}

// SubscriptionStatusesBundle abstracts management of subscription-statuses bundle.
type SubscriptionStatusesBundle struct {
	base.BaseManagerBundle
	Objects []*appsv1alpha1.SubscriptionStatus `json:"objects"`
}

// GetObjects return all the objects that the bundle holds.
func (bundle *SubscriptionStatusesBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
