package placement

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/generic"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/hub-of-hubs/agent/pkg/transport/producer"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

const (
	placementDecisionsSyncLog = "placement-decisions-sync"
)

// AddPlacementDecisionsController adds placement-decision controller to the manager.
func AddPlacementDecisionsController(mgr ctrl.Manager, transport producer.Producer, leafHubName string,
	incarnation uint64, _ *corev1.ConfigMap, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.PlacementDecision{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementDecisionMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, nil),
			func() bool { return true }),
	} // bundle predicate - always send placement decision.

	if err := generic.NewGenericStatusSyncController(mgr, placementDecisionsSyncLog, transport, bundleCollection,
		createObjFunction, nil, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add placement decisions controller to the manager - %w", err)
	}

	return nil
}
