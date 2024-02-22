package hubcluster

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/cache"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddHubClusterInfoSyncer creates a controller and adds it to the manager.
// this controller is responsible for syncing the hub cluster status.
// right now, it only syncs the openshift console url.
func AddHubClusterInfoSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	leafHubName := config.GetLeafHubName()
	clusterInfoBundle := cluster.NewAgentHubClusterInfoBundle(leafHubName)

	transportKey := fmt.Sprintf("%s.%s", leafHubName, constants.HubClusterInfoMsgKey)
	bundlePredicate := func() bool { return true }
	bundleEntry := generic.NewSharedBundleEntry(transportKey, clusterInfoBundle, bundlePredicate)
	objectCollection := []bundle.SharedBundleObject{
		cluster.NewHubClusterInfoClaimObject(),
		cluster.NewHubClusterInfoRouteObject(),
	}

	cache.RegistToCache(constants.HubClusterInfoMsgKey, clusterInfoBundle)
	return generic.NewGenericSharedBundleSyncer(mgr, producer, bundleEntry, objectCollection,
		config.GetHubClusterInfoDuration)
}

// func LaunchGenericEventSyncer(name string, mgr ctrl.Manager, eventControllers []EventController,
// 	producer transport.Producer, intervalFunc func() time.Duration, emitter Emitter,
// )

func LaunchHubClusterInfoSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	eventData := &cluster.HubClusterInfo{}
	return generic.LaunchGenericEventSyncer(
		"status.hub_cluster_info",
		mgr,
		[]generic.EventController{
			NewInfoClusterClaimController(eventData),
			NewInfoRouteController(eventData),
		},
		producer,
		config.GetHubClusterInfoDuration,
		generic.NewGenericEmitter(enum.HubClusterInfoType, eventData),
	)
}
