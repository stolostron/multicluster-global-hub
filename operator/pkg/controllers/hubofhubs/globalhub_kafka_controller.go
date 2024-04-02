// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubofhubs

import (
	"context"
	"fmt"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/deployer"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/renderer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var kafkaController *KafkaController

type KafkaController struct {
	Log                 logr.Logger
	mgr                 ctrl.Manager
	globalHubReconciler *MulticlusterGlobalHubReconciler
	conn                *transport.ConnCredential
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// get the mcgh cr name and then trigger the globalhub reconciler
	mgh := &globalhubv1alpha4.MulticlusterGlobalHub{}
	err := r.mgr.GetClient().Get(ctx, config.GetMGHNamespacedName(), mgh)
	if err != nil {
		r.Log.Error(err, "failed to get MulticlusterGlobalHub")
		return ctrl.Result{}, err
	}
	err = r.renderKafkaMetricsResources(mgh)
	if err != nil {
		return ctrl.Result{}, err
	}
	r.conn, err = r.globalHubReconciler.ReconcileTransport(ctx, mgh, transport.StrimziTransporter)
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
	reconciler *MulticlusterGlobalHubReconciler,
) (*KafkaController, error) {
	r := &KafkaController{
		Log:                 ctrl.Log.WithName("kafka-controller"),
		mgr:                 mgr,
		globalHubReconciler: reconciler,
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

// renderKafkaMetricsResources renders the kafka podmonitor and metrics
func (r *KafkaController) renderKafkaMetricsResources(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	if mgh.Spec.EnableMetrics {
		// render the kafka objects
		kafkaRenderer, kafkaDeployer := renderer.NewHoHRenderer(fs), deployer.NewHoHDeployer(r.mgr.GetClient())
		kafkaObjects, err := kafkaRenderer.Render("manifests/kafka", "",
			func(profile string) (interface{}, error) {
				return struct {
					Namespace string
				}{
					Namespace: utils.GetDefaultNamespace(),
				}, nil
			})
		if err != nil {
			return fmt.Errorf("failed to render kafka manifests: %w", err)
		}
		// create restmapper for deployer to find GVR
		dc, err := discovery.NewDiscoveryClientForConfig(r.mgr.GetConfig())
		if err != nil {
			return err
		}
		mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

		if err = manipulateObj(kafkaObjects, mgh, kafkaDeployer, mapper, r.mgr.GetScheme()); err != nil {
			return fmt.Errorf("failed to create/update kafka objects: %w", err)
		}
	}
	return nil
}

type kafkaCRDController struct {
	mgr                ctrl.Manager
	globaHubReconciler *MulticlusterGlobalHubReconciler
}

func (c *kafkaCRDController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	if kafkaController == nil {
		reconclier, err := startKafkaController(ctx, c.mgr, c.globaHubReconciler)
		if err != nil {
			return ctrl.Result{}, err
		}
		kafkaController = reconclier
	}
	return ctrl.Result{}, nil
}

// this controller is used to watch the Kafka/KafkaTopic/KafkaUser crd
// if the crd exists, then add controllers to the manager dynamically
func addKafkaCRDController(mgr ctrl.Manager, reconciler *MulticlusterGlobalHubReconciler) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&apiextensionsv1.CustomResourceDefinition{}, builder.WithPredicates(predicate.Funcs{
			// trigger the reconciler only if the crd is created
			CreateFunc: func(e event.CreateEvent) bool {
				return e.Object.GetName() == "kafkas.kafka.strimzi.io"
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
		Complete(&kafkaCRDController{
			mgr:                mgr,
			globaHubReconciler: reconciler,
		})
}
