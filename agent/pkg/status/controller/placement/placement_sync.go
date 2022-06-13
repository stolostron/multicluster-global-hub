package placement

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/generic"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/hub-of-hubs/agent/pkg/transport/producer"
	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	placementSyncLog = "placement-sync"
)

// AddPlacementsController adds placement controller to the manager.
func AddPlacementsController(mgr ctrl.Manager, transport producer.Producer, leafHubName string,
	incarnation uint64, _ *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.Placement{} }

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacement),
			func() bool { return true }),
	} // bundle predicate - always send placements.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, placementSyncLog, transport, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add placements controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacement(object bundle.Object) {
	placement, ok := object.(*clustersv1beta1.Placement)
	if !ok {
		panic("Wrong instance passed to clean placement function, not a placement")
	}
	// clean spec. no need for it.
	placement.Spec = clustersv1beta1.PlacementSpec{}
}
