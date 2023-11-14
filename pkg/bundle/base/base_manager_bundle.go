package base

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

type BaseManagerBundle struct {
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (baseBundle *BaseManagerBundle) GetLeafHubName() string {
	return baseBundle.LeafHubName
}

// GetVersion returns the bundle version.
func (baseBundle *BaseManagerBundle) GetVersion() *metadata.BundleVersion {
	return baseBundle.BundleVersion
}

func (baseBundle *BaseManagerBundle) SetVersion(version *metadata.BundleVersion) {
	baseBundle.BundleVersion = version
}
