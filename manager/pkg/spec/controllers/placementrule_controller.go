// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controllers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

func AddPlacementRuleController(mgr ctrl.Manager, specDB specdb.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&placementrulev1.PlacementRule{}).
		WithEventFilter(GlobalResourcePredicate()).
		Complete(&genericSpecController{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            logger.ZapLogger("placementrules-spec-syncer"),
			tableName:      "placementrules",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &placementrulev1.PlacementRule{} },
			cleanObject:    cleanPlacementRuleObject,
			areEqual:       arePlacementRulesEqual,
		}); err != nil {
		return fmt.Errorf("failed to add placement rule controller to the manager: %w", err)
	}

	return nil
}

func cleanPlacementRuleObject(instance client.Object) {
	placementRule, ok := instance.(*placementrulev1.PlacementRule)

	if !ok {
		panic("wrong instance passed to cleanPlacementRuleStatus: not a PlacementRule")
	}
	// reset to empty so that the placementrule scheduler can take over
	placementRule.Spec.SchedulerName = ""

	placementRule.Status = placementrulev1.PlacementRuleStatus{}
}

func arePlacementRulesEqual(instance1, instance2 client.Object) bool {
	placementRule1, ok1 := instance1.(*placementrulev1.PlacementRule)
	placementRule2, ok2 := instance2.(*placementrulev1.PlacementRule)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(placementRule1.Spec, placementRule2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
