package grc

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ bundle.ManagerBundle = (*LocalPolicyBundle)(nil)

// LocalPolicyBundle abstracts management of local policies spec bundle.
type LocalPolicyBundle struct {
	base.BaseManagerBundle
	Objects []*policyv1.Policy `json:"objects"`
}

// NewManagerLocalPolicyBundle creates a new instance of LocalPolicySpecBundle.
func NewManagerLocalPolicyBundle() bundle.ManagerBundle {
	return &LocalPolicyBundle{}
}

// GetObjects returns the objects in the bundle.
func (bundle *LocalPolicyBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
