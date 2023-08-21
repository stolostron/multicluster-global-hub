package placement

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	placementDecisionsSyncLog = "placement-decisions-sync"
)

// AddPlacementDecisionsController adds placement-decision controller to the manager.
func AddPlacementDecisionsController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.PlacementDecision{} }
	leafHubName := agentstatusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementDecisionMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send placement decision.

	if err := generic.NewGenericStatusSyncController(mgr, placementDecisionsSyncLog, producer, bundleCollection,
		createObjFunction, nil, agentstatusconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add placement decisions controller to the manager - %w", err)
	}

	return nil
}
