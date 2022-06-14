// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	configv1 "github.com/stolostron/hub-of-hubs/pkg/apis/config/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	client "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	hohSystemNamespace = "hoh-system"
)

func AddHubOfHubsConfigController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&configv1.Config{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return object.GetNamespace() == hohSystemNamespace
		})).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("hoh-configs-spec-syncer"),
			tableName:      "configs",
			finalizerName:  hohCleanupFinalizer,
			createInstance: func() client.Object { return &configv1.Config{} },
			cleanStatus:    cleanConfigStatus,
			areEqual:       areConfigsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add hoh config controller to the manager: %w", err)
	}

	return nil
}

func cleanConfigStatus(instance client.Object) {
	config, ok := instance.(*configv1.Config)

	if !ok {
		panic("wrong instance passed to cleanConfigStatus: not configv1.Config")
	}

	config.Status = configv1.ConfigStatus{}
}

func areConfigsEqual(instance1, instance2 client.Object) bool {
	config1, ok1 := instance1.(*configv1.Config)
	config2, ok2 := instance2.(*configv1.Config)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(config1.Spec, config2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
