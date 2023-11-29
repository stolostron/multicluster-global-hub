package hubcluster

import (
	"time"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ bundle.ObjectHandler = (*hubClusterRouteHandler)(nil)

type hubClusterRouteHandler struct{}

func NewHubClusterInfoRouteHandler() *hubClusterRouteHandler {
	return &hubClusterRouteHandler{}
}

func (h *hubClusterRouteHandler) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
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
}

func (h *hubClusterRouteHandler) CreateObject() bundle.Object {
	return &routev1.Route{}
}

func (h *hubClusterRouteHandler) SyncIntervalFunc() func() time.Duration {
	return config.GetHubClusterInfoDuration
}

func (h *hubClusterRouteHandler) BundleUpdate(obj bundle.Object, b bundle.BaseAgentBundle) {
	hubClusterBundle, ok := ensureBundle(b)
	if !ok {
		return
	}

	var routeURL string
	route, ok := obj.(*routev1.Route)
	if ok {
		if len(route.Spec.Host) != 0 {
			routeURL = "https://" + route.Spec.Host
		}
		if route.GetName() == constants.OpenShiftConsoleRouteName && hubClusterBundle.Objects[0].ConsoleURL != routeURL {
			hubClusterBundle.Objects[0].ConsoleURL = routeURL
			hubClusterBundle.GetVersion().Incr()
		}
		if route.GetName() == constants.ObservabilityGrafanaRouteName &&
			hubClusterBundle.Objects[0].GrafanaURL != routeURL {
			hubClusterBundle.Objects[0].GrafanaURL = routeURL
			hubClusterBundle.GetVersion().Incr()
		}
	}
}

func (h *hubClusterRouteHandler) BundleDelete(obj bundle.Object, b bundle.BaseAgentBundle) {
	hubClusterBundle, ok := ensureBundle(b)
	if !ok {
		return
	}

	if obj.GetName() == constants.OpenShiftConsoleRouteName && hubClusterBundle.Objects[0].ConsoleURL != "" {
		hubClusterBundle.Objects[0].ConsoleURL = ""
		hubClusterBundle.GetVersion().Incr()
	}
	if obj.GetName() == constants.ObservabilityGrafanaRouteName && hubClusterBundle.Objects[0].GrafanaURL != "" {
		hubClusterBundle.Objects[0].GrafanaURL = ""
		hubClusterBundle.GetVersion().Incr()
	}
}
