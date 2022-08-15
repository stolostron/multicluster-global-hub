package bundle

import "github.com/stolostron/multicluster-globalhub/pkg/bundle/status"

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClustersPerPolicyBundle() Bundle {
	return &ClustersPerPolicyBundle{}
}

// ClustersPerPolicyBundle abstracts management of clusters per policy bundle.
type ClustersPerPolicyBundle struct {
	status.BaseClustersPerPolicyBundle
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (bundle *ClustersPerPolicyBundle) GetLeafHubName() string {
	return bundle.LeafHubName
}

// GetObjects returns the objects in the bundle.
func (bundle *ClustersPerPolicyBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(bundle.Objects))
	for i, obj := range bundle.Objects {
		result[i] = obj
	}

	return result
}

// GetVersion returns the bundle version.
func (bundle *ClustersPerPolicyBundle) GetVersion() *status.BundleVersion {
	return bundle.BundleVersion
}
