package localplacement

import (
	"fmt"

	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
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
	localPlacementRuleStatusSyncLog = "local-placement-rule-status-sync"
)

// AddLocalPlacementRulesController adds a new local placement rules controller.
func AddLocalPlacementRulesController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunc := func() bundle.Object { return &placementrulesv1.PlacementRule{} }
	leafHubName := config.GetLeafHubName()

	localPlacementRuleTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPlacementRulesMsgKey)

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(localPlacementRuleTransportKey,
			genericbundle.NewStatusGenericBundle(leafHubName, cleanPlacementRule),
			func() bool { // bundle predicate
				return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue
			}),
	}
	// controller predicate
	localPlacementRulePredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})

	if err := generic.NewStatusGenericSyncer(mgr, localPlacementRuleStatusSyncLog, producer, bundleCollection,
		createObjFunc, localPlacementRulePredicate, config.GetPolicyDuration); err != nil {
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
