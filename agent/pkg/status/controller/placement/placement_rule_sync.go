package placement

import (
	"fmt"

	placementrulesV1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	placementRuleSyncLog = "placement-rules-sync"
	PlacementRuleMsgKey  = "PlacementRule"
)

// AddPlacementRulesController adds placement-rule controller to the manager.
func AddPlacementRulesController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &placementrulesV1.PlacementRule{} }
	leafHubName := agentstatusconfig.GetLeafHubName()

	// TODO datatypes.PlacementRuleMsgKey
	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, PlacementRuleMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, cleanPlacementRule),
			func() bool { return true }),
	} // bundle predicate - always send placement rules.

	// TODO datatypes.OriginOwnerReferenceAnnotation
	ownerRefAnnotationPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, placementRuleSyncLog, producer, bundleCollection,
		createObjFunction, ownerRefAnnotationPredicate, agentstatusconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add placement rules controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacementRule(object bundle.Object) {
	placementrule, ok := object.(*placementrulesV1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement-rule function, not a placement-rule")
	}
	// clean spec. no need for it.
	placementrule.Spec = placementrulesV1.PlacementRuleSpec{}
}
