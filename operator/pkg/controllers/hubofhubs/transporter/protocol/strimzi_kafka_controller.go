// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	if mgh == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	needRequeue, err := r.trans.EnsureKafka()
	if err != nil {
		return ctrl.Result{}, err
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	kafkaReady, reconcileErr := r.trans.kafkaClusterReady()
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}
	if !kafkaReady {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// use the client ca to sign the csr for the managed hubs
	if err := config.SetClientCA(r.trans.ctx, r.trans.mgh.Namespace, KafkaClusterName,
		r.trans.manager.GetClient()); err != nil {
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

	// Update status to mgh so that it can trigger a GlobalHubReconciler
	if err := config.UpdateCondition(ctx, r.GetClient(), mgh, metav1.Condition{
		Type:               "KafkaClusterReady",
		Status:             metav1.ConditionTrue,
		Reason:             "KafkaClusterIsReady",
		Message:            "Kafka cluster is ready",
		LastTransitionTime: metav1.Time{Time: time.Now()},
	}); err != nil {
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
