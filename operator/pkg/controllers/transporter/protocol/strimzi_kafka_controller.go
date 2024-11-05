// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
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

type KafkaStatus struct {
	kakfaReason  string
	kafkaMessage string
	kafkaReady   bool
}

// KafkaController reconciles the kafka crd
type KafkaController struct {
	ctrl.Manager
	trans       *strimziTransporter
	kafkaStatus KafkaStatus
}

// +kubebuilder:rbac:groups=operators.coreos.com,resources=subscriptions,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,verbs=delete
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkas;kafkatopics;kafkausers;kafkanodepools,verbs=get;create;list;watch;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules;podmonitors,verbs=get;create;delete;update;list;watch
func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// If mgh is deleting, return
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.GetClient())
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) || mgh.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	var reconcileErr error
	defer func() {
		err = config.UpdateMGHComponent(ctx, r.GetClient(),
			r.getKafkaComponentStatus(reconcileErr, r.kafkaStatus),
		)
		if err != nil {
			klog.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	needRequeue, err := r.trans.EnsureKafka()
	if err != nil {
		return ctrl.Result{}, err
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.kafkaStatus, reconcileErr = r.trans.kafkaClusterReady()
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}
	if !r.kafkaStatus.kafkaReady {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// use the client ca to sign the csr for the managed hubs
	if err := config.SetKafkaClientCA(r.trans.ctx, r.trans.mgh.Namespace, KafkaClusterName,
		r.trans.manager.GetClient()); err != nil {
		return ctrl.Result{}, err
	}
	// update the transporter
	config.SetTransporter(r.trans)

	// update the transport connection
	conn, needRequeue, err := getManagerTransportConn(r.trans, DefaultGlobalHubKafkaUserName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
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
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(config.MGHPred)).
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

func getManagerTransportConn(trans *strimziTransporter, kafkaUserSecret string) (
	*transport.KafkaConfig, bool, error,
) {
	// set transporter connection
	var conn *transport.KafkaConfig
	var err error

	// boostrapServer, clusterId, clusterCA
	conn, err = trans.getConnCredentailByCluster()
	if err != nil {
		klog.Info("waiting the kafka cluster credential to be ready...", "message", err.Error())
		return conn, true, err
	}
	// topics
	conn.SpecTopic = config.GetSpecTopic()
	conn.StatusTopic = config.ManagerStatusTopic()
	// clientCert and clientCA
	if err := trans.loadUserCredentail(kafkaUserSecret, conn); err != nil {
		klog.Info("waiting the kafka user credential to be ready...", "message", err.Error())
		return conn, true, err
	}
	return conn, false, nil
}

func (r *KafkaController) getKafkaComponentStatus(reconcileErr error, kafkaClusterStatus KafkaStatus,
) v1alpha4.StatusCondition {
	if reconcileErr != nil {
		return v1alpha4.StatusCondition{
			Kind:    "TransportConnection",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  config.RECONCILE_ERROR,
			Message: reconcileErr.Error(),
		}
	}
	if !kafkaClusterStatus.kafkaReady {
		return v1alpha4.StatusCondition{
			Kind:    "Kafka",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  kafkaClusterStatus.kakfaReason,
			Message: kafkaClusterStatus.kafkaMessage,
		}
	}
	if config.GetTransporterConn() == nil {
		return v1alpha4.StatusCondition{
			Kind:    "TransportConnection",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  "TransportConnectionNotSet",
			Message: "Transport connection is null",
		}
	}
	return v1alpha4.StatusCondition{
		Kind:    "TransportConnection",
		Name:    config.COMPONENTS_KAFKA_NAME,
		Type:    config.COMPONENTS_AVAILABLE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "TransportConnectionSet",
		Message: "Transport connection has set",
	}
}
