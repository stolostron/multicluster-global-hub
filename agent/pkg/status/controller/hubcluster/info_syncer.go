package hubcluster

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

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
