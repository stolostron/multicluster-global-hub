// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package controller

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	appv1beta1 "sigs.k8s.io/application/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func AddApplicationController(mgr ctrl.Manager, specDB db.SpecDB) error {
	if err := ctrl.NewControllerManagedBy(mgr).
		For(&appv1beta1.Application{}).
		Complete(&genericSpecToDBReconciler{
			client:         mgr.GetClient(),
			specDB:         specDB,
			log:            ctrl.Log.WithName("applications-spec-syncer"),
			tableName:      "applications",
			finalizerName:  constants.GlobalHubCleanupFinalizer,
			createInstance: func() client.Object { return &appv1beta1.Application{} },
			cleanStatus:    cleanApplicationStatus,
			areEqual:       areApplicationsEqual,
		}); err != nil {
		return fmt.Errorf("failed to add application controller to the manager: %w", err)
	}

	return nil
}

func cleanApplicationStatus(instance client.Object) {
	application, ok := instance.(*appv1beta1.Application)
	if !ok {
		panic("wrong instance passed to cleanApplicationStatus: not an Application")
	}

	application.Status = appv1beta1.ApplicationStatus{}
}

func areApplicationsEqual(instance1, instance2 client.Object) bool {
	application1, ok1 := instance1.(*appv1beta1.Application)
	application2, ok2 := instance2.(*appv1beta1.Application)

	if !ok1 || !ok2 {
		return false
	}

	specMatch := equality.Semantic.DeepEqual(application1.Spec, application2.Spec)
	annotationsMatch := equality.Semantic.DeepEqual(instance1.GetAnnotations(), instance2.GetAnnotations())
	labelsMatch := equality.Semantic.DeepEqual(instance1.GetLabels(), instance2.GetLabels())

	return specMatch && annotationsMatch && labelsMatch
}
