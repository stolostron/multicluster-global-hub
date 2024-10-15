package managedhub

import (
	routev1 "github.com/openshift/api/route/v1"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func LaunchHubClusterInfoSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	eventData := &cluster.HubClusterInfo{}
	return generic.LaunchMultiObjectSyncer(
		"status.hub_cluster_info",
		mgr,
		[]generic.ControllerHandler{
			{
				Controller: generic.NewGenericController(
					func() client.Object { return &clustersv1alpha1.ClusterClaim{} },
					predicate.NewPredicateFuncs(func(object client.Object) bool {
						return object.GetName() == "id.k8s.io"
					})),
				Handler: &infoClusterClaimHandler{eventData},
			},
			{
				Controller: generic.NewGenericController(
					func() client.Object { return &routev1.Route{} },
					predicate.NewPredicateFuncs(func(object client.Object) bool {
						if object.GetNamespace() == constants.OpenShiftConsoleNamespace &&
							object.GetName() == constants.OpenShiftConsoleRouteName {
							return true
						}
						if object.GetNamespace() == constants.ObservabilityNamespace &&
							object.GetName() == constants.ObservabilityGrafanaRouteName {
							return true
						}
						return false
					})),
				Handler: &infoRouteHandler{eventData},
			},
		},
		producer,
		configmap.GetHubClusterInfoDuration,
		generic.NewGenericEmitter(enum.HubClusterInfoType),
	)
}

// 1. Use ClusterClaim to update the HubClusterInfo
type infoClusterClaimHandler struct {
	evtData cluster.HubClusterInfoBundle
}

func (p *infoClusterClaimHandler) Get() interface{} {
	return p.evtData
}

func (p *infoClusterClaimHandler) Update(obj client.Object) bool {
	clusterClaim, ok := obj.(*clustersv1alpha1.ClusterClaim)
	if !ok {
		return false
	}

	oldClusterID := p.evtData.ClusterId

	if clusterClaim.Name == "id.k8s.io" {
		p.evtData.ClusterId = clusterClaim.Spec.Value
	}
	// If no ClusterId, do not send the bundle
	if p.evtData.ClusterId == "" {
		return false
	}

	return oldClusterID != p.evtData.ClusterId
}

func (p *infoClusterClaimHandler) Delete(obj client.Object) bool {
	// do nothing
	return false
}

// 2. Use Route to update the HubClusterInfo
type infoRouteHandler struct {
	evtData cluster.HubClusterInfoBundle
}

func (p *infoRouteHandler) Get() interface{} {
	return p.evtData
}

func (p *infoRouteHandler) Update(obj client.Object) bool {
	route, ok := obj.(*routev1.Route)
	if !ok {
		return false
	}

	var newURL string
	updated := false
	if len(route.Spec.Host) != 0 {
		newURL = "https://" + route.Spec.Host
	}
	if route.GetName() == constants.OpenShiftConsoleRouteName && p.evtData.ConsoleURL != newURL {
		p.evtData.ConsoleURL = newURL
		updated = true
	}
	if route.GetName() == constants.ObservabilityGrafanaRouteName && p.evtData.GrafanaURL != newURL {
		p.evtData.GrafanaURL = newURL
		updated = true
	}
	return updated
}

func (p *infoRouteHandler) Delete(obj client.Object) bool {
	updated := false
	if obj.GetName() == constants.OpenShiftConsoleRouteName && p.evtData.ConsoleURL != "" {
		p.evtData.ConsoleURL = ""
		updated = true
	}
	if obj.GetName() == constants.ObservabilityGrafanaRouteName && p.evtData.GrafanaURL != "" {
		p.evtData.GrafanaURL = ""
		updated = true
	}
	return updated
}
