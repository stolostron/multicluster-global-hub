// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
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
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		return r.pruneStrimziResources(ctx)
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

func waitManagerTransportConn(ctx context.Context, trans *strimziTransporter, kafkaUserSecret string) (
	*transport.KafkaConfig, error,
) {
	// set transporter connection
	var conn *transport.KafkaConfig
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

func (r *KafkaController) pruneStrimziResources(ctx context.Context) (ctrl.Result, error) {
	klog.Infof("Remove strimzi resources")
	listOpts := []client.ListOption{
		client.HasLabels{constants.GlobalHubOwnerLabelKey},
	}
	kafkaUserList := &kafkav1beta2.KafkaUserList{}
	klog.Infof("Delete kafkaUsers")
	if err := r.GetClient().List(ctx, kafkaUserList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}
	for idx := range kafkaUserList.Items {
		klog.Infof("Delete kafka user %v", kafkaUserList.Items[idx].Name)
		if err := r.GetClient().Delete(ctx, &kafkaUserList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	kafkaTopicList := &kafkav1beta2.KafkaTopicList{}
	klog.Infof("Delete kafkaTopics")

	if err := r.GetClient().List(ctx, kafkaTopicList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}
	for idx := range kafkaTopicList.Items {
		klog.Infof("Delete kafka topic %v", kafkaTopicList.Items[idx].Name)
		if err := r.GetClient().Delete(ctx, &kafkaTopicList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	// Wait kafkatopic is removed
	if err := r.GetClient().List(ctx, kafkaTopicList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	if len(kafkaTopicList.Items) != 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	kafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.trans.kafkaClusterName,
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	klog.Infof("Delete kafka cluster %v", kafka.Name)

	if err := r.GetClient().Delete(ctx, kafka); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	klog.Infof("kafka cluster deleted")

	kafkaSub := &subv1alpha1.Subscription{}
	err := r.GetClient().Get(ctx, types.NamespacedName{
		Namespace: utils.GetDefaultNamespace(),
		Name:      r.trans.subName,
	}, kafkaSub)
	if err != nil {
		klog.Errorf("Failed to get strimzi subscription, err:%v", err)
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if kafkaSub.Status.InstalledCSV != "" {
		kafkaCsv := &subv1alpha1.ClusterServiceVersion{
			ObjectMeta: metav1.ObjectMeta{
				Name:      kafkaSub.Status.InstalledCSV,
				Namespace: utils.GetDefaultNamespace(),
			},
		}
		klog.Infof("Delete kafka csv %v", kafkaCsv.Name)
		if err := r.GetClient().Delete(ctx, kafkaCsv); err != nil {
			return ctrl.Result{}, err
		}
		klog.Infof("kafka csv deleted")
	}

	if err := r.GetClient().Delete(ctx, kafkaSub); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	klog.Infof("kafka subscription deleted")
	return ctrl.Result{}, nil
}
