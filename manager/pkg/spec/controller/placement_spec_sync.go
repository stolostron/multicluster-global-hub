// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddPlacementController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.Placement{}).
		WithEventFilter(GlobalResourcePredicate()).
		Complete(&genericSpecController{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("placements-spec-syncer"),
			tableName:      "placements",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &clusterv1beta1.Placement{} },
			cleanObject:    cleanPlacementObject,
			areEqual:       arePlacementsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add placement controller to the manager: %w", err)
	}

	return nil
}

func cleanPlacementObject(instance client.Object) {
	placement, ok := instance.(*clusterv1beta1.Placement)

	if !ok {
		panic("wrong instance passed to cleanPlacementStatus: not a Placement")
	}

	if placement.Annotations != nil {
		// remove the annotation so that the placement controller can take over
		delete(placement.Annotations, clusterv1beta1.PlacementDisableAnnotation)
	}
	placement.Status = clusterv1beta1.PlacementStatus{}
}

func arePlacementsEqual(instance1, instance2 client.Object) bool {
	placement1, ok1 := instance1.(*clusterv1beta1.Placement)
	placement2, ok2 := instance2.(*clusterv1beta1.Placement)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(placement1.Spec, placement2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
