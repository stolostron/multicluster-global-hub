package hubcluster

import (
	routev1 "github.com/openshift/api/route/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var _ generic.EventController = &infoRouteController{}

type infoRouteController struct {
	generic.Controller
	evtData cluster.HubClusterInfoData
}

func NewInfoRouteController(eventData cluster.HubClusterInfoData) generic.EventController {
	instance := func() client.Object { return &routev1.Route{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if object.GetNamespace() == constants.OpenShiftConsoleNamespace &&
			object.GetName() == constants.OpenShiftConsoleRouteName {
			return true
		}
		if object.GetNamespace() == constants.ObservabilityNamespace &&
			object.GetName() == constants.ObservabilityGrafanaRouteName {
			return true
		}
		return false
	})

	return &infoRouteController{
		Controller: generic.NewGenericController(instance, predicate),
		evtData:    eventData,
	}
}

func (p *infoRouteController) Update(obj client.Object) bool {

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

func (p *infoRouteController) Delete(obj client.Object) bool {
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

func (p *infoRouteController) Data() interface{} {
	return p.evtData
}
