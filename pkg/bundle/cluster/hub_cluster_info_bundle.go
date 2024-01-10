package cluster

import (
	"sync"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

var (
	_ bundle.ManagerBundle   = (*HubClusterInfoBundle)(nil)
	_ bundle.BaseAgentBundle = (*HubClusterInfoBundle)(nil)
)

// HubClusterInfoBundle holds information for leaf hub cluster info status bundle.
type HubClusterInfoBundle struct {
	base.BaseHubClusterInfoBundle
	lock sync.Mutex
}

// LeafHubClusterInfoStatusBundle creates a new instance of LeafHubClusterInfoStatusBundle.
func NewAgentHubClusterInfoBundle(leafHubName string) *HubClusterInfoBundle {
	return &HubClusterInfoBundle{
		BaseHubClusterInfoBundle: base.BaseHubClusterInfoBundle{
			Objects:       make([]*base.HubClusterInfo, 0),
			LeafHubName:   leafHubName,
			BundleVersion: metadata.NewBundleVersion(),
		},
	}
}

func NewManagerHubClusterInfoBundle() bundle.ManagerBundle {
	return &HubClusterInfoBundle{}
}

// Manager - GetObjects return all the objects that the bundle holds.
func (b *HubClusterInfoBundle) GetObjects() []interface{} {
	result := make([]interface{}, len(b.Objects))
	for i, obj := range b.Objects {
		result[i] = obj
	}
	return result
}

// Manager - GetLeafHubName returns the leaf hub name that sent the bundle.
func (b *HubClusterInfoBundle) GetLeafHubName() string {
	return b.LeafHubName
}

// Manager
func (b *HubClusterInfoBundle) SetVersion(version *metadata.BundleVersion) {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.BundleVersion = version
}

// GetBundleVersion function to get bundle version.
func (b *HubClusterInfoBundle) GetVersion() *metadata.BundleVersion {
	return b.BundleVersion
}
