// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func AddPlacementRuleController(mgr ctrl.Manager, specDB db.SpecDB) error {
	placementRulePredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.HubOfHubsLocalResource,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&placementrulev1.PlacementRule{}).
		WithEventFilter(placementRulePredicate).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("placementrules-spec-syncer"),
			tableName:      "placementrules",
			finalizerName:  hohCleanupFinalizer,
			createInstance: func() client.Object { return &placementrulev1.PlacementRule{} },
			cleanStatus:    cleanPlacementRuleStatus,
			areEqual:       arePlacementRulesEqual,
		}); err != nil {
		return fmt.Errorf("failed to add placement rule controller to the manager: %w", err)
	}

	return nil
}

func cleanPlacementRuleStatus(instance client.Object) {
	placementRule, ok := instance.(*placementrulev1.PlacementRule)

	if !ok {
		panic("wrong instance passed to cleanPlacementRuleStatus: not a PlacementRule")
	}

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
