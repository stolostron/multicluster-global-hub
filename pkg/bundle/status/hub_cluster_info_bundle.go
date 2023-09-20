package status

// LeafHubClusterInfo manages leaf hub cluster info.
type LeafHubClusterInfo struct {
	LeafHubName string `json:"leafHubName"`
	ConsoleURL  string `json:"consoleURL"`
	GrafanaURL  string `json:"grafanaURL"`
}

// HubClusterInfoBundle the bundle for the hub cluster info.
type HubClusterInfoBundle struct {
	Objects       []*LeafHubClusterInfo `json:"objects"`
	LeafHubName   string                `json:"leafHubName"`
	BundleVersion *BundleVersion        `json:"bundleVersion"`
}

// GetObjects return all the objects that the bundle holds.
func (baseBundle *HubClusterInfoBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(baseBundle.Objects))
	for i, obj := range baseBundle.Objects {
		result[i] = obj
	}

	return result
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (baseBundle *HubClusterInfoBundle) GetLeafHubName() string {
	return baseBundle.LeafHubName
}

// GetVersion returns the bundle version.
func (baseBundle *HubClusterInfoBundle) GetVersion() *BundleVersion {
	return baseBundle.BundleVersion
}

func (baseBundle *HubClusterInfoBundle) SetVersion(version *BundleVersion) {
	baseBundle.BundleVersion = version
}

func (LeafHubClusterInfo) TableName() string {
	return "status.leaf_hubs"
}
