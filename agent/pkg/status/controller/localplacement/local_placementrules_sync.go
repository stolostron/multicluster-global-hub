package localplacement

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	localPlacementRuleStatusSyncLog = "local-placement-rule-status-sync"
)

// AddLocalPlacementRulesController adds a new local placement rules controller.
func AddLocalPlacementRulesController(mgr ctrl.Manager, transport producer.Producer, leafHubName string,
	incarnation uint64, hubOfHubsConfig *corev1.ConfigMap, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	createObjFunc := func() bundle.Object { return &placementrulesv1.PlacementRule{} }

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPlacementRulesMsgKey)

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localPlacementRuleTransportKey,
			bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPlacementRule),
			func() bool { // bundle predicate
				return hubOfHubsConfig.Data["enableLocalPolicies"] == "true"
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
