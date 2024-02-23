package placement

import (
	"context"
	"fmt"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// AddPlacementSyncer adds placement controller to the manager.
func AddPlacementSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &clustersv1beta1.Placement{} }
	leafHubName := statusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementMsgKey),
			genericbundle.NewGenericStatusBundle(leafHubName, cleanPlacement),
			func() bool { return true }),
	} // bundle predicate - always send placements.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	return generic.NewGenericStatusSyncer(mgr, "placement-sync", producer, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, statusconfig.GetPolicyDuration)
}

func cleanPlacement(object bundle.Object) {
	placement, ok := object.(*clustersv1beta1.Placement)
	if !ok {
		panic("Wrong instance passed to clean placement function, not a placement")
	}
	// clean spec. no need for it.
	placement.Spec = clustersv1beta1.PlacementSpec{}
}

func LaunchPlacementSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer) error {

	// controller config
	instance := func() client.Object { return &clustersv1beta1.Placement{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter config
	globalPlacementEmitter := generic.ObjectEmitterWrapper(enum.PlacementSpecType,
		func(obj client.Object) bool {
			return agentConfig.EnableGlobalResource &&
				utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
		},
		func(obj client.Object) {
			placement, ok := obj.(*clustersv1beta1.Placement)
			if !ok {
				panic("Wrong instance passed to clean placement function, not a placement")
			}
			placement.Spec = clustersv1beta1.PlacementSpec{}
		},
	)

	// syncer
	name := "status.placement"
	syncInterval := statusconfig.GetPolicyDuration

	return generic.LaunchGenericObjectSyncer(
		name,
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		syncInterval,
		[]generic.ObjectEmitter{
			globalPlacementEmitter,
		})
}
