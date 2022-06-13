// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func AddPlacementBindingController(mgr ctrl.Manager, specDB db.SpecDB) error {
	placementBindingPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.HubOfHubsLocalResource,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&policyv1.PlacementBinding{}).
		WithEventFilter(placementBindingPredicate).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("placementbindings-spec-syncer"),
			tableName:      "placementbindings",
			finalizerName:  hohCleanupFinalizer,
			createInstance: func() client.Object { return &policyv1.PlacementBinding{} },
			cleanStatus:    cleanPlacementBindingStatus,
			areEqual:       arePlacementBindingsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add placement binding controller to the manager: %w", err)
	}

	return nil
}

func cleanPlacementBindingStatus(instance client.Object) {
	placementBinding, ok := instance.(*policyv1.PlacementBinding)

	if !ok {
		panic("wrong instance passed to cleanPlacementBindingStatus: not a PlacementBinding")
	}

	placementBinding.Status = policyv1.PlacementBindingStatus{}
}

func arePlacementBindingsEqual(instance1, instance2 client.Object) bool {
	placementBinding1, ok1 := instance1.(*policyv1.PlacementBinding)
	placementBinding2, ok2 := instance2.(*policyv1.PlacementBinding)

	if !ok1 || !ok2 {
		return false
	}

	placementRefMatch := equality.Semantic.DeepEqual(placementBinding1.PlacementRef, placementBinding2.PlacementRef)
	subjectsMatch := equality.Semantic.DeepEqual(placementBinding1.Subjects, placementBinding2.Subjects)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return placementRefMatch && subjectsMatch && annotationsMatch && labelsMatch
}
