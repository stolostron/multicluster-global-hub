// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// ReconcileResources use to render all the kafka resources when its resources is changed, and also get the new conn.
type ReconcileResources func(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub) error

// KafkaController reconciles the kafka crd
type KafkaController struct {
	mgr                ctrl.Manager
	reconcileResources ReconcileResources
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	mgh := &v1alpha4.MulticlusterGlobalHub{}
	if err := r.mgr.GetClient().Get(ctx, config.GetMGHNamespacedName(), mgh); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if err := r.reconcileResources(ctx, mgh); err != nil {
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

func StartKafkaController(mgr ctrl.Manager,
	kafkaResourceReconcile ReconcileResources,
) (*KafkaController, error) {
	r := &KafkaController{
		mgr:                mgr,
		reconcileResources: kafkaResourceReconcile,
	}

	err := ctrl.NewControllerManagedBy(mgr).
		Named("kafka_controller").
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
	klog.Info("kafka controller is started")
	return r, nil
}
