package status

// LeafHubClusterInfo manages leaf hub cluster info.
type LeafHubClusterInfo struct {
	LeafHubName string `json:"leafHubName"`
	ConsoleURL  string `json:"consoleURL"`
}

// BaseLeafHubClusterInfoStatusBundle the bundle for the hub cluster info.
type BaseLeafHubClusterInfoStatusBundle struct {
	Objects       []*LeafHubClusterInfo `json:"objects"`
	LeafHubName   string                `json:"leafHubName"`
	BundleVersion *BundleVersion        `json:"bundleVersion"`
}

// GetObjects return all the objects that the bundle holds.
func (baseBundle *BaseLeafHubClusterInfoStatusBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(baseBundle.Objects))
	for i, obj := range baseBundle.Objects {
		result[i] = obj
	}

	return result
}

// GetLeafHubName returns the leaf hub name that sent the bundle.
func (baseBundle *BaseLeafHubClusterInfoStatusBundle) GetLeafHubName() string {
	return baseBundle.LeafHubName
}

// GetVersion returns the bundle version.
func (baseBundle *BaseLeafHubClusterInfoStatusBundle) GetVersion() *BundleVersion {
	return baseBundle.BundleVersion
}

func (baseBundle *BaseLeafHubClusterInfoStatusBundle) SetVersion(version *BundleVersion) {
	baseBundle.BundleVersion = version
}

func (LeafHubClusterInfo) TableName() string {
	return "status.leaf_hubs"
}
