package placement

import (
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	placementSyncLog = "placement-sync"
)

// AddPlacementsController adds placement controller to the manager.
func AddPlacementsController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.Placement{} }
	leafHubName := config.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementMsgKey),
			genericbundle.NewStatusGenericBundle(leafHubName, cleanPlacement),
			func() bool { return true }),
	} // bundle predicate - always send placements.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	return generic.NewStatusGenericSyncer(mgr, placementSyncLog, producer, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, config.GetPolicyDuration)
}

func cleanPlacement(object bundle.Object) {
	placement, ok := object.(*clustersv1beta1.Placement)
	if !ok {
		panic("Wrong instance passed to clean placement function, not a placement")
	}
	// clean spec. no need for it.
	placement.Spec = clustersv1beta1.PlacementSpec{}
}
