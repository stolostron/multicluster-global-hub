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
)

func AddManagedClusterSetController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1beta1.ManagedClusterSet{}).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("managedclustersets-spec-syncer"),
			tableName:      "managedclustersets",
			finalizerName:  hohCleanupFinalizer,
			createInstance: func() client.Object { return &clusterv1beta1.ManagedClusterSet{} },
			cleanStatus:    cleanManagedClusterSetStatus,
			areEqual:       areManagedClusterSetsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add managed cluster set controller to the manager: %w", err)
	}

	return nil
}

func cleanManagedClusterSetStatus(instance client.Object) {
	managedClusterSet, ok := instance.(*clusterv1beta1.ManagedClusterSet)

	if !ok {
		panic("wrong instance passed to cleanManagedClusterSetStatus: not a ManagedClusterSet")
	}

	managedClusterSet.Status = clusterv1beta1.ManagedClusterSetStatus{}
}

func areManagedClusterSetsEqual(instance1, instance2 client.Object) bool {
	managedClusterSet1, ok1 := instance1.(*clusterv1beta1.ManagedClusterSet)
	managedClusterSet2, ok2 := instance2.(*clusterv1beta1.ManagedClusterSet)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(managedClusterSet1.Spec, managedClusterSet2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
