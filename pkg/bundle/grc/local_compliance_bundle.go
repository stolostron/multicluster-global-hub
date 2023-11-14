package grc

import "github.com/stolostron/multicluster-global-hub/pkg/bundle"

var _ bundle.ManagerBundle = (*LocalComplianceBundle)(nil)

// NewLocalClustersPerPolicyBundle creates a new instance of LocalClustersPerPolicyBundle.
func NewManagerLocalComplianceBundle() bundle.ManagerBundle {
	return &LocalComplianceBundle{}
}

// LocalClustersPerPolicyBundle abstracts management of local clusters per policy bundle.
type LocalComplianceBundle struct{ ComplianceBundle }
