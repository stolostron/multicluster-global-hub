package hubcluster

import (
	"sync"

	routev1 "github.com/openshift/api/route/v1"

	agentbundle "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// LeafHubClusterInfoStatusBundle creates a new instance of LeafHubClusterInfoStatusBundle.
func NewLeafHubClusterInfoStatusBundle(leafHubName string) agentbundle.Bundle {
	return &LeafHubClusterInfoStatusBundle{
		HubClusterInfoBundle: statusbundle.HubClusterInfoBundle{
			Objects:       make([]*statusbundle.LeafHubClusterInfo, 0),
			LeafHubName:   leafHubName,
			BundleVersion: statusbundle.NewBundleVersion(),
		},
		lock: sync.Mutex{},
	}
}

// LeafHubClusterInfoStatusBundle holds information for leaf hub cluster info status bundle.
type LeafHubClusterInfoStatusBundle struct {
	statusbundle.HubClusterInfoBundle
	lock sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *LeafHubClusterInfoStatusBundle) UpdateObject(object agentbundle.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	var routeURL string
	route := object.(*routev1.Route)
	if len(route.Spec.Host) != 0 {
		routeURL = "https://" + route.Spec.Host
	}

	if len(bundle.Objects) == 0 {
		if route.GetName() == constants.OpenShiftConsoleRouteName {
			bundle.Objects = []*statusbundle.LeafHubClusterInfo{
				{
					LeafHubName: bundle.LeafHubName,
					ConsoleURL:  routeURL,
				},
			}
		} else if route.GetName() == constants.ObservabilityGrafanaRouteName {
			bundle.Objects = []*statusbundle.LeafHubClusterInfo{
				{
					LeafHubName: bundle.LeafHubName,
					GrafanaURL:  routeURL,
				},
			}
		}
	} else {
		if route.GetName() == constants.OpenShiftConsoleRouteName {
			bundle.Objects[0].ConsoleURL = routeURL
		} else if route.GetName() == constants.ObservabilityGrafanaRouteName {
			bundle.Objects[0].GrafanaURL = routeURL
		}
	}
	bundle.BundleVersion.Incr()
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *LeafHubClusterInfoStatusBundle) DeleteObject(object agentbundle.Object) {
	// do nothing
}

// GetBundleVersion function to get bundle version.
func (bundle *LeafHubClusterInfoStatusBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}
