package placement

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddPlacementDecisionsController adds placement-decision controller to the manager.
func AddPlacementDecisionsController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.PlacementDecision{} }
	leafHubName := config.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementDecisionMsgKey),
			genericbundle.NewGenericStatusBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send placement decision.

	return generic.NewGenericStatusSyncer(mgr, "placement-decisions-sync", producer, bundleCollection,
		createObjFunction, nil, agentstatusconfig.GetPolicyDuration)
}
