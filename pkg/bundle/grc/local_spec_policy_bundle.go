package grc

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ bundle.ManagerBundle = (*LocalSpecPolicyBundle)(nil)

// LocalSpecPolicyBundle abstracts management of local policies spec bundle.
type LocalSpecPolicyBundle struct {
	base.BaseManagerBundle
	Objects []*policyv1.Policy `json:"objects"`
}

// NewManagerLocalSpecPolicyBundle creates a new instance of LocalPolicySpecBundle.
func NewManagerLocalSpecPolicyBundle() bundle.ManagerBundle {
	return &LocalSpecPolicyBundle{}
}

// GetObjects returns the objects in the bundle.
func (bundle *LocalSpecPolicyBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
