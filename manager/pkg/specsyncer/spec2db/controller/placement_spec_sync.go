// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddPlacementController(mgr ctrl.Manager, specDB db.SpecDB) error {
	placementPredicate, _ := predicate.LabelSelectorPredicate(metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      constants.GlobalHubLocalResource,
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
		},
	})
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1alpha1.Placement{}).
		WithEventFilter(placementPredicate).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("placements-spec-syncer"),
			tableName:      "placements",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &clusterv1alpha1.Placement{} },
			cleanStatus:    cleanPlacementStatus,
			areEqual:       arePlacementsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add placement controller to the manager: %w", err)
	}

	return nil
}

func cleanPlacementStatus(instance client.Object) {
	placement, ok := instance.(*clusterv1alpha1.Placement)

	if !ok {
		panic("wrong instance passed to cleanPlacementStatus: not a Placement")
	}

	placement.Status = clusterv1alpha1.PlacementStatus{}
}

func arePlacementsEqual(instance1, instance2 client.Object) bool {
	placement1, ok1 := instance1.(*clusterv1alpha1.Placement)
	placement2, ok2 := instance2.(*clusterv1alpha1.Placement)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(placement1.Spec, placement2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
