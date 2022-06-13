package bundle

import (
	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
)

// NewSubscriptionReportsBundle creates a new instance of SubscriptionReportsBundle.
func NewSubscriptionReportsBundle() Bundle {
	return &SubscriptionReportsBundle{}
}

// SubscriptionReportsBundle abstracts management of subscription-reports bundle.
type SubscriptionReportsBundle struct {
	baseBundle
	Objects []*appsv1alpha1.SubscriptionReport `json:"objects"`
}

// GetObjects return all the objects that the bundle holds.
func (bundle *SubscriptionReportsBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
