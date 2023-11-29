package base

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"

// HubClusterInfo manages leaf hub cluster info.
type HubClusterInfo struct {
	ConsoleURL string `json:"consoleURL"`
	GrafanaURL string `json:"grafanaURL"`
	ClusterId  string `json:"clusterId"`
}

// BaseHubClusterInfoBundle the bundle for the hub cluster info.
type BaseHubClusterInfoBundle struct {
	Objects       []*HubClusterInfo       `json:"objects"`
	LeafHubName   string                  `json:"leafHubName"`
	BundleVersion *metadata.BundleVersion `json:"bundleVersion"`
}
