package controlinfo

import (
	"sync"

	agentBundle "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewControlInfoBundle creates a new instance of Bundle.
func NewControlInfoBundle(leafHubName string) agentBundle.Bundle {
	return &ControlInfoBundle{
		LeafHubName:   leafHubName,
		BundleVersion: status.NewBundleVersion(),
		lock:          sync.Mutex{},
	}
}

// ControlInfoBundle holds control info passed from LH to HoH.
type ControlInfoBundle struct {
	LeafHubName   string                `json:"leafHubName"`
	BundleVersion *status.BundleVersion `json:"bundleVersion"`
	lock          sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ControlInfoBundle) UpdateObject(_ agentBundle.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	bundle.BundleVersion.Incr()
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ControlInfoBundle) DeleteObject(_ agentBundle.Object) {
	// do nothing
}

// GetBundleVersion function to get bundle version.
func (bundle *ControlInfoBundle) GetBundleVersion() *status.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}
