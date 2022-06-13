package localplacement

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/generic"
	"github.com/stolostron/hub-of-hubs/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/hub-of-hubs/agent/pkg/transport/producer"
	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	localPlacementRuleStatusSyncLog = "local-placement-rule-status-sync"
)

// AddLocalPlacementRulesController adds a new local placement rules controller.
func AddLocalPlacementRulesController(mgr ctrl.Manager, transport producer.Producer, leafHubName string,
	incarnation uint64, hubOfHubsConfig *configv1.Config, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	createObjFunc := func() bundle.Object { return &placementrulesv1.PlacementRule{} }

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPlacementRulesMsgKey)

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localPlacementRuleTransportKey,
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacementRule),
			func() bool { // bundle predicate
				return hubOfHubsConfig.Spec.EnableLocalPolicies
			}),
	}
	// controller predicate
	localPlacementRulePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPlacementRuleStatusSyncLog, transport, bundleCollection,
		createObjFunc, localPlacementRulePredicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local placement rules controller to the manager - %w", err)
	}

	return nil
}

func cleanPlacementRule(object bundle.Object) {
	placement, ok := object.(*placementrulesv1.PlacementRule)
	if !ok {
		panic("Wrong instance passed to clean placement rule function, not appsv1.PlacementRule")
	}

	placement.Status = placementrulesv1.PlacementRuleStatus{}
}
