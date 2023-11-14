package hubcluster

import (
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddHubClusterInfoSyncer creates a controller and adds it to the manager.
// this controller is responsible for syncing the hub cluster status.
// right now, it only syncs the openshift console url.
func AddHubClusterInfoSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &routev1.Route{} }
	leafHubName := config.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.HubClusterInfoMsgKey),
			cluster.NewAgentHubClusterInfoBundle(leafHubName),
			func() bool { return true }),
	} // bundle predicate - always send subscription status.

	hubClusterInfoPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return (object.GetNamespace() == constants.OpenShiftConsoleNamespace &&
			object.GetName() == constants.OpenShiftConsoleRouteName) ||
			(object.GetNamespace() == constants.ObservabilityNamespace &&
				object.GetName() == constants.ObservabilityGrafanaRouteName)
	})

	return generic.NewStatusGenericSyncer(mgr, "hub-cluster-status-sync", producer, bundleCollection,
		createObjFunction, hubClusterInfoPredicate, config.GetHubClusterInfoDuration)
}
