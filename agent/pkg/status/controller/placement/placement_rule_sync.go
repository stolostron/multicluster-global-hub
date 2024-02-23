package placement

import (
	"context"
	"fmt"

	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
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

// AddPlacementRulesController adds placement-rule controller to the manager.
func AddPlacementRulesController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &placementrulesv1.PlacementRule{} }
	leafHubName := statusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.PlacementRuleMsgKey),
			genericbundle.NewGenericStatusBundle(leafHubName, cleanPlacementRule),
			func() bool { return true }),
	} // bundle predicate - always send placement rules.

	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	return generic.NewGenericStatusSyncer(mgr, "placement-rules-sync", producer, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, statusconfig.GetPolicyDuration)
}

func cleanPlacementRule(object bundle.Object) {
	placementrule, ok := object.(*placementrulesv1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement-rule function, not a placement-rule")
	}
	// clean spec. no need for it.
	placementrule.Spec = placementrulesv1.PlacementRuleSpec{}
}

func LaunchPlacementRuleSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer) error {

	// controller config
	instance := func() client.Object { return &placementrulesv1.PlacementRule{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })
	controller := generic.NewGenericController(instance, predicate)

	// emitter config
	tweakFunc := func(obj client.Object) {
		placementrule, ok := obj.(*placementrulesv1.PlacementRule)
		if !ok {
			panic("Wrong instance passed to clean placement-rule function, not a placement-rule")
		}
		placementrule.Spec = placementrulesv1.PlacementRuleSpec{}
	}
	globalPlacementRuleEmitter := generic.ObjectEmitterWrapper(enum.PlacementRuleSpecType,
		func(obj client.Object) bool {
			return agentConfig.EnableGlobalResource &&
				utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
		}, tweakFunc)

	localPlacementRuleEmitter := generic.ObjectEmitterWrapper(enum.LocalPlacementRuleSpecType,
		func(obj client.Object) bool {
			return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue &&
				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) // local resource
		}, tweakFunc)

	// syncer
	name := "status.placement_decision"
	syncInterval := statusconfig.GetPolicyDuration

	return generic.LaunchGenericObjectSyncer(
		name,
		mgr,
		controller,
		producer,
		syncInterval,
		[]generic.ObjectEmitter{
			globalPlacementRuleEmitter,
			localPlacementRuleEmitter,
		})
}
