// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// kafkaReconcileHandler use to render all the kafka resources when its resources is changed, and also get the new conn.
type kafkaReconcileHandler func() (*transport.ConnCredential, error)

// KafkaController reconciles the kafka crd
type KafkaController struct {
	Log              logr.Logger
	mgr              ctrl.Manager
	reconcileHandler kafkaReconcileHandler
	conn             *transport.ConnCredential
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// get the mcgh cr name and then trigger the globalhub reconciler
	var err error
	r.conn, err = r.reconcileHandler()
	if err != nil {
		r.Log.Error(err, "failed to get connection from kafka reconciler")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

var kafkaPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return e.Object.GetNamespace() == utils.GetDefaultNamespace()
	},
}

// initialize the kafka transporter and then start the kafka/user/topic controller
func startKafkaController(ctx context.Context, mgr ctrl.Manager,
	getConnFunc kafkaReconcileHandler,
) (*KafkaController, error) {
	r := &KafkaController{
		Log:              ctrl.Log.WithName("kafka-controller"),
		mgr:              mgr,
		reconcileHandler: getConnFunc,
	}
	_, err := r.Reconcile(ctx, ctrl.Request{})
	if err != nil {
		return nil, err
	}

	err = ctrl.NewControllerManagedBy(mgr).
		Named("middleware_kafka_controller").
		Watches(&kafkav1beta2.Kafka{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaUser{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaTopic{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Complete(r)
	if err != nil {
		return nil, err
	}
	r.Log.Info("kafka controller is started")
	return r, nil
}
