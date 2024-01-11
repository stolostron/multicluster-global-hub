package cluster

import (
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

var _ bundle.ManagerBundle = (*HubClusterHeartbeatBundle)(nil)
var _ bundle.BaseAgentBundle = (*HubClusterHeartbeatBundle)(nil)

// LocalPolicyBundle abstracts management of local policies spec bundle.
type HubClusterHeartbeatBundle struct {
	base.BaseManagerBundle
}

// NewManagerHubClusterHeartbeatBundle creates a new instance of HubClusterHeartbeatBundle.
func NewManagerHubClusterHeartbeatBundle() bundle.ManagerBundle {
	return &HubClusterHeartbeatBundle{}
}

// NewAgentHubClusterHeartbeatBundle creates a new instance of HubClusterHeartbeatBundle.
func NewAgentHubClusterHeartbeatBundle(leafHubName string) *HubClusterHeartbeatBundle {
	return &HubClusterHeartbeatBundle{
		base.BaseManagerBundle{
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		},
	}
}

// GetObjects returns the objects in the bundle.
func (bundle *HubClusterHeartbeatBundle) GetObjects() []interface{} {
	return nil
}
