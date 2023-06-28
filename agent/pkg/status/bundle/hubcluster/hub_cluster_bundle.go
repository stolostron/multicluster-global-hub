package hubcluster

import (
	"sync"

	routev1 "github.com/openshift/api/route/v1"

	bundlepkg "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// LeafHubClusterInfoStatusBundle creates a new instance of LeafHubClusterInfoStatusBundle.
func NewLeafHubClusterInfoStatusBundle(leafHubName string, incarnation uint64) bundlepkg.Bundle {
	return &LeafHubClusterInfoStatusBundle{
		BaseLeafHubClusterInfoStatusBundle: statusbundle.BaseLeafHubClusterInfoStatusBundle{
			Objects:       make([]*statusbundle.LeafHubClusterInfo, 0),
			LeafHubName:   leafHubName,
			BundleVersion: statusbundle.NewBundleVersion(incarnation, 0),
		},
		lock: sync.Mutex{},
	}
}

// LeafHubClusterInfoStatusBundle holds information for leaf hub cluster info status bundle.
type LeafHubClusterInfoStatusBundle struct {
	statusbundle.BaseLeafHubClusterInfoStatusBundle
	lock sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *LeafHubClusterInfoStatusBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	route := object.(*routev1.Route)

	bundle.Objects = append(bundle.Objects,
		&statusbundle.LeafHubClusterInfo{
			LeafHubName: bundle.LeafHubName,
			ConsoleURL:  "https://" + route.Spec.Host,
		})
	bundle.BundleVersion.Generation++
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *LeafHubClusterInfoStatusBundle) DeleteObject(object bundlepkg.Object) {
	// do nothing
}

// GetBundleVersion function to get bundle version.
func (bundle *LeafHubClusterInfoStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}
