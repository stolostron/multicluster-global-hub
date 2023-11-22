package grc

import "github.com/stolostron/multicluster-global-hub/pkg/bundle"

var (
	_ bundle.AgentBundle            = (*CompleteComplianceBundle)(nil)
	_ bundle.ManagerDependantBundle = (*CompleteComplianceBundle)(nil)
)

// NewLocalClustersPerPolicyBundle creates a new instance of LocalClustersPerPolicyBundle.
func NewManagerLocalCompleteComplianceBundle() bundle.ManagerBundle {
	return &LocalCompleteComplianceBundle{}
}

// LocalClustersPerPolicyBundle abstracts management of local clusters per policy bundle.
type LocalCompleteComplianceBundle struct{ CompleteComplianceBundle }
