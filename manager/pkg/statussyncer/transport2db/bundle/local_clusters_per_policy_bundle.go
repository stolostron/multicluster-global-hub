package bundle

// NewLocalClustersPerPolicyBundle creates a new instance of LocalClustersPerPolicyBundle.
func NewLocalClustersPerPolicyBundle() Bundle {
	return &LocalClustersPerPolicyBundle{}
}

// LocalClustersPerPolicyBundle abstracts management of local clusters per policy bundle.
type LocalClustersPerPolicyBundle struct{ ClustersPerPolicyBundle }
