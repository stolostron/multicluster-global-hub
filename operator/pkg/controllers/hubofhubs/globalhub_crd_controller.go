// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type crdController struct {
	mgr        ctrl.Manager
	reconciler *MulticlusterGlobalHubReconciler
	crdErr     error
}

var once sync.Once

func (c *crdController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	once.Do(func() {
		// start to watch kafka/kafkatopic/kafkauser custom resource controller
		_, c.crdErr = StartMiddlewareController(ctx, c.mgr, c.reconciler)
	})
	if c.crdErr != nil {
		// if the crd controller fails to start, then return the error
		_, c.crdErr = StartMiddlewareController(ctx, c.mgr, c.reconciler)
		return ctrl.Result{}, c.crdErr
	}
	return ctrl.Result{}, nil
}

// this controller is used to watch the Kafka/KafkaTopic/KafkaUser crd
// if the crd exists, then add controllers to the manager dynamically
func StartCRDController(mgr ctrl.Manager, reconciler *MulticlusterGlobalHubReconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}, builder.WithPredicates(predicate.Funcs{
			// trigger the reconciler only if the crd is created
			CreateFunc: func(e event.CreateEvent) bool {
				if e.Object.GetName() == "kafkas.kafka.strimzi.io" || e.Object.GetName() == "kafkatopics.kafka.strimzi.io" ||
					e.Object.GetName() == "kafkausers.kafka.strimzi.io" {
					return true
				}
				return false
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return false
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return false
			},
		})).
		Complete(&crdController{
			mgr:        mgr,
			reconciler: reconciler,
		})
}
