// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddPlacementBindingController(mgr ctrl.Manager, specDB specdb.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&policyv1.PlacementBinding{}).
		WithEventFilter(GlobalResourcePredicate()).
		Complete(&genericSpecController{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("placementbindings-spec-syncer"),
			tableName:      "placementbindings",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &policyv1.PlacementBinding{} },
			cleanObject:    cleanPlacementBindingStatus,
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
