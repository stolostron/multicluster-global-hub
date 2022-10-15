package bundle

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"

// NewLocalClustersPerPolicyBundle creates a new instance of LocalClustersPerPolicyBundle.
func NewLocalClustersPerPolicyBundle() status.Bundle {
	return &LocalClustersPerPolicyBundle{}
}

// LocalClustersPerPolicyBundle abstracts management of local clusters per policy bundle.
type LocalClustersPerPolicyBundle struct{ ClustersPerPolicyBundle }
