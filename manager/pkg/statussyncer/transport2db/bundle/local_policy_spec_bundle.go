package bundle

import (
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

// NewLocalPolicySpecBundle creates a new instance of LocalPolicySpecBundle.
func NewLocalPolicySpecBundle() Bundle {
	return &LocalPolicySpecBundle{}
}

// LocalPolicySpecBundle abstracts management of local policies spec bundle.
type LocalPolicySpecBundle struct {
	baseBundle
	Objects []*policyv1.Policy `json:"objects"`
}

// GetObjects returns the objects in the bundle.
func (bundle *LocalPolicySpecBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}
