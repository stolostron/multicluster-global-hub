package hubcluster

import (
	"fmt"

	routev1 "github.com/openshift/api/route/v1"
	clustersv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
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

var bundleCollection []*generic.BundleEntry

// AddHubClusterInfoSyncer creates a controller and adds it to the manager.
// this controller is responsible for syncing the hub cluster status.
// right now, it only syncs the openshift console url.
func AddHubClusterInfoSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &routev1.Route{} }
	leafHubName := config.GetLeafHubName()

	bundleCollection = []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.HubClusterInfoMsgKey),
			cluster.NewAgentHubClusterInfoBundle(leafHubName),
			func() bool { return true }),
	} // bundle predicate - always send subscription status.

	hubClusterInfoPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
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

	err := generic.NewGenericStatusSyncer(mgr, "hub-cluster-status-sync", producer, bundleCollection,
		createObjFunction, hubClusterInfoPredicate, config.GetHubClusterInfoDuration)
	if err != nil {
		return err
	}

	createClaimObjFunction := func() bundle.Object { return &clustersv1alpha1.ClusterClaim{} }
	hubClusterClaimPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == "id.k8s.io"
	})
	err = generic.NewGenericStatusSyncer(mgr, "hub-cluster-status-sync", producer, bundleCollection,
		createClaimObjFunction, hubClusterClaimPredicate, config.GetHubClusterInfoDuration)
	if err != nil {
		return err
	}
	return nil
}

func IncreaseBundleVersion() {
	for _, entry := range bundleCollection {
		// update in each bundle from the collection according to their order.
		entry.Bundle.IncrVersion()
	}
}
