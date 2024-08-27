// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var manifests embed.FS

// KafkaController reconciles the kafka crd
type KafkaController struct {
	ctrl.Manager
	trans *strimziTransporter
}

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// If mgh is deleting, return
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	if err := r.trans.EnsureKafka(); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.trans.kafkaClusterReady(); err != nil {
		return ctrl.Result{}, err
	}

	// use the client ca to sign the csr for the managed hubs
	if err := config.SetClientCA(r.trans.ctx, r.trans.mgh.Namespace, KafkaClusterName, r.trans.runtimeClient); err != nil {
		return ctrl.Result{}, err
	}
	// update the transporter
	config.SetTransporter(r.trans)

	// update the transport connection
	conn, err := waitManagerTransportConn(ctx, r.trans, DefaultGlobalHubKafkaUserName)
	if err != nil {
		return ctrl.Result{}, err
	}
	config.SetTransporterConn(conn)

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

var mghPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return true
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return false
	},
}

func StartKafkaController(ctx context.Context, mgr ctrl.Manager, transporter transport.Transporter) (
	*KafkaController, error,
) {
	r := &KafkaController{
		Manager: mgr,
		trans:   transporter.(*strimziTransporter),
	}

	// even if the following controller will reconcile the transport, but it's asynchoronized
	err := ctrl.NewControllerManagedBy(mgr).
		Named("strimzi_controller").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(mghPred)).
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

func waitManagerTransportConn(ctx context.Context, trans *strimziTransporter, kafkaUserSecret string) (
	*transport.KafkaConnCredential, error,
) {
	// set transporter connection
	var conn *transport.KafkaConnCredential
	var err error
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			// boostrapServer, clusterId, clusterCA
			conn, err = trans.getConnCredentailByCluster()
			if err != nil {
				klog.Info("waiting the kafka cluster credential to be ready...", "message", err.Error())
				return false, err
			}
			// topics
			conn.SpecTopic = config.GetSpecTopic()
			conn.StatusTopic = config.ManagerStatusTopic()
			// clientCert and clientCA
			if err := trans.loadUserCredentail(kafkaUserSecret, conn); err != nil {
				klog.Info("waiting the kafka user credential to be ready...", "message", err.Error())
				return false, err
			}
			return true, nil
		})
	if err != nil {
		return nil, err
	}
	return conn, nil
}
