package grc

import "github.com/stolostron/multicluster-global-hub/pkg/bundle"

var _ bundle.ManagerBundle = (*LocalCompleteComplianceBundle)(nil)

// NewLocalClustersPerPolicyBundle creates a new instance of LocalClustersPerPolicyBundle.
func NewManagerLocalCompleteComplianceBundle() bundle.ManagerBundle {
	return &LocalCompleteComplianceBundle{}
}

// LocalClustersPerPolicyBundle abstracts management of local clusters per policy bundle.
type LocalCompleteComplianceBundle struct{ CompleteComplianceBundle }
