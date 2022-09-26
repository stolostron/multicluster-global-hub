// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddHubOfHubsConfigController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetNamespace() == constants.HohSystemNamespace &&
				object.GetName() == constants.HoHConfigName
		})).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("hoh-configs-spec-syncer"),
			tableName:      "configs",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &corev1.ConfigMap{} },
			cleanObject:    cleanConfigStatus,
			areEqual:       areConfigsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add hoh config controller to the manager: %w", err)
	}

	return nil
}

func cleanConfigStatus(instance client.Object) {
	_, ok := instance.(*corev1.ConfigMap)

	if !ok {
		panic("wrong instance passed to cleanConfigStatus: not corev1.ConfigMap")
	}
}

func areConfigsEqual(instance1, instance2 client.Object) bool {
	config1, ok1 := instance1.(*corev1.ConfigMap)
	config2, ok2 := instance2.(*corev1.ConfigMap)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(config1.Data, config2.Data)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
