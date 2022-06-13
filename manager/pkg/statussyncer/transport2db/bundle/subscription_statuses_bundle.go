package bundle

import (
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
)

// NewSubscriptionStatusesBundle creates a new instance of SubscriptionStatusesBundle.
func NewSubscriptionStatusesBundle() Bundle {
	return &SubscriptionStatusesBundle{}
}

// SubscriptionStatusesBundle abstracts management of subscription-statuses bundle.
type SubscriptionStatusesBundle struct {
	baseBundle
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
