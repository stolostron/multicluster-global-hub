// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type crdController struct {
	mgr        ctrl.Manager
	reconciler *MulticlusterGlobalHubReconciler
}

func (c *crdController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// start to watch kafka/kafkatopic/kafkauser custom resource controller
	if err := StartMiddlewareController(c.mgr, c.reconciler); err != nil {
		return ctrl.Result{}, err
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
