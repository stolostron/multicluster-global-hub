package bundle

import "github.com/stolostron/multicluster-globalhub/pkg/bundle/status"

type baseBundle struct {
	LeafHubName   string                `json:"leafHubName"`
	BundleVersion *status.BundleVersion `json:"bundleVersion"`
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (baseBundle *baseBundle) GetLeafHubName() string {
	return baseBundle.LeafHubName
}

// GetVersion returns the bundle version.
func (baseBundle *baseBundle) GetVersion() *status.BundleVersion {
	return baseBundle.BundleVersion
}
