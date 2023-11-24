package base

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// Agent to Manager
type BaseGenericStatusBundle struct {
	Objects       []bundle.Object         `json:"objects"`
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}
