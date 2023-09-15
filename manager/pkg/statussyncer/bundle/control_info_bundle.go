package bundle

import "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"

// NewHubClusterInfoBundle creates a new instance of ControlInfoBundle.
func NewHubClusterInfoBundle() status.Bundle {
	return &HubClusterHeartbeatBundle{}
}

// HubClusterHeartbeatBundle abstracts management of control info bundle.
type HubClusterHeartbeatBundle struct {
	baseBundle
}

// GetObjects returns the objects in the bundle.
func (bundle *HubClusterHeartbeatBundle) GetObjects() []interface{} {
	return nil
}

func (bundle *HubClusterHeartbeatBundle) SetVersion(version *status.BundleVersion) {
	bundle.BundleVersion = version
}
