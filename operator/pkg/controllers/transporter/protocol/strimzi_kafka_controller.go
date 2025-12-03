// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"fmt"
	"reflect"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	operatorutils "github.com/stolostron/multicluster-global-hub/operator/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

//go:embed manifests
var manifests embed.FS

var (
	startedKafkaController = false
	isResourceRemoved      = false
	lastTransportConn      *transport.KafkaConfig
)

var log = logger.DefaultZapLogger()

type KafkaStatus struct {
	kafkaReason  string
	kafkaMessage string
	kafkaReady   bool
}

// KafkaController reconciles the kafka crd
type KafkaController struct {
	c           client.Client
	kafkaStatus KafkaStatus
}

func IsResourceRemoved() bool {
	if config.IsBYOKafka() {
		return true
	}
	return isResourceRemoved
}

// +kubebuilder:rbac:groups=operators.coreos.com,resources=subscriptions,verbs=get;create;delete;update;list;watch
// +kubebuilder:rbac:groups=operators.coreos.com,resources=clusterserviceversions,verbs=get;delete;list;watch
// +kubebuilder:rbac:groups=kafka.strimzi.io,resources=kafkas;kafkatopics;kafkausers;kafkanodepools,verbs=get;create;list;watch;update;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors;prometheusrules;podmonitors,verbs=get;create;delete;update;list;watch

func (r *KafkaController) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	// If mgh is deleting, return
	mgh, err := config.GetMulticlusterGlobalHub(ctx, r.c)
	if err != nil {
		return ctrl.Result{}, err
	}
	if mgh == nil || config.IsPaused(mgh) {
		return ctrl.Result{}, nil
	}
	if mgh.DeletionTimestamp != nil {
		if !config.GetGlobalhubAgentRemoved() {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		lastTransportConn = nil
		return r.pruneStrimziResources(ctx)
	}
	isResourceRemoved = false
	if !mgh.Spec.EnableMetrics {
		err = operatorutils.PruneMetricsResources(ctx, r.c,
			map[string]string{
				constants.GlobalHubMetricsLabel: "strimzi",
			})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// wait for the strimzi transport to be initialized
	if strimziTransport == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	defer func() {
		err = config.UpdateMGHComponent(ctx, r.c, r.getKafkaComponentStatus(err, r.kafkaStatus), false)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()

	err = strimziTransport.Validate()
	if err != nil {
		log.Errorf("failed to validate strimzi transport, err:%v", err)
		return ctrl.Result{}, err
	}

	err = strimziTransport.Initialize()
	if err != nil {
		log.Errorf("failed to initialize strimzi transport, err:%v", err)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.kafkaStatus, err = strimziTransport.kafkaClusterReady()
	if err != nil {
		log.Errorf("failed to get kafka cluster status, err:%v", err)
		return ctrl.Result{}, err
	}
	if !r.kafkaStatus.kafkaReady {
		log.Infof("waiting for the kafka cluster to be ready, err:%s", r.kafkaStatus.kafkaMessage)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// use the client ca to sign the csr for the managed hubs
	if err := config.SetKafkaClientCA(strimziTransport.ctx, strimziTransport.mgh.Namespace,
		KafkaClusterName, strimziTransport.manager.GetClient()); err != nil {
		return ctrl.Result{}, err
	}
	// update the transport connection, if conn is nil, requeue to let it ready
	conn, err := getManagerTransportConn(strimziTransport)
	if err != nil {
		return ctrl.Result{}, err
	}
	if conn == nil {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// update the kafka component status timestamp to trigger the mgh controller to reconcile the transportConfig secret
	if !reflect.DeepEqual(conn, lastTransportConn) {
		log.Infof("update transport conn: spec(%s), status(%s)", conn.SpecTopic, conn.StatusTopic)
		// Update MGH annotation to trigger MetaController and ManagerReconciler reconciliation
		// Note: controller-runtime's For() method does not trigger reconciliation for status-only updates,
		// even with custom predicates. Therefore, we update an annotation (metadata change) to ensure
		// the MetaController gets notified and can reconcile the transportConfig secret.
		// Use retry mechanism to handle optimistic concurrency conflicts
		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			// Fetch the latest version of MGH to avoid conflicts
			if err := r.c.Get(ctx, config.GetMGHNamespacedName(), mgh); err != nil {
				return err
			}
			if mgh.Annotations == nil {
				mgh.Annotations = make(map[string]string)
			}
			mgh.Annotations[operatorconstants.AnnotationMGHTransportUpdate] = time.Now().Format(time.RFC3339)
			return r.c.Update(ctx, mgh)
		})
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to update mgh annotation to trigger reconciliation, err:%v", err)
		}
		lastTransportConn = conn
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

var kafkaUserPred = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return false
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectNew.GetName() != DefaultGlobalHubKafkaUserName {
			return false
		}
		return e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		if e.Object.GetName() != DefaultGlobalHubKafkaUserName {
			return false
		}
		return e.Object.GetNamespace() == utils.GetDefaultNamespace()
	},
}

func StartKafkaController(ctx context.Context, mgr ctrl.Manager) error {
	if startedKafkaController {
		return nil
	}
	log.Info("start kafka controller")
	r := &KafkaController{
		c: mgr.GetClient(),
	}

	// even if the following controller will reconcile the transport, but it's asynchoronized
	err := ctrl.NewControllerManagedBy(mgr).
		Named("strimzi_controller").
		For(&v1alpha4.MulticlusterGlobalHub{}, builder.WithPredicates(config.MGHPred)).
		Watches(&kafkav1beta2.Kafka{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Watches(&kafkav1beta2.KafkaUser{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaUserPred)).
		Watches(&kafkav1beta2.KafkaTopic{},
			&handler.EnqueueRequestForObject{}, builder.WithPredicates(kafkaPred)).
		Complete(r)
	if err != nil {
		return err
	}
	startedKafkaController = true
	log.Info("kafka controller is started")
	return nil
}

func getManagerTransportConn(trans *strimziTransporter) (
	*transport.KafkaConfig, error,
) {
	// bootstrapServer, clusterId, clusterCA
	conn, err := trans.getConnCredentialByCluster()
	if err != nil {
		log.Infow("waiting the kafka cluster credential to be ready...", "message", err.Error())
		return conn, err
	}

	// topics
	conn.SpecTopic = config.GetSpecTopic()
	conn.StatusTopic = config.ManagerStatusTopic()
	// clientCert and clientCA
	if err := trans.loadUserCredential(DefaultGlobalHubKafkaUserName, conn); err != nil {
		log.Infow("waiting the kafka user credential to be ready...", "message", err.Error())
		return nil, nil
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
			Reason:  kafkaClusterStatus.kafkaReason,
			Message: kafkaClusterStatus.kafkaMessage,
		}
	}
	if lastTransportConn == nil {
		return v1alpha4.StatusCondition{
			Kind:    "TransportConnection",
			Name:    config.COMPONENTS_KAFKA_NAME,
			Type:    config.COMPONENTS_AVAILABLE,
			Status:  config.CONDITION_STATUS_FALSE,
			Reason:  "TransportConnectionNotReady",
			Message: "Transport connection is not ready",
		}
	}
	return v1alpha4.StatusCondition{
		Kind:    "TransportConnection",
		Name:    config.COMPONENTS_KAFKA_NAME,
		Type:    config.COMPONENTS_AVAILABLE,
		Status:  config.CONDITION_STATUS_TRUE,
		Reason:  "TransportConnectionReady",
		Message: "Transport connection is ready",
	}
}

func (r *KafkaController) pruneStrimziResources(ctx context.Context) (ctrl.Result, error) {
	log.Infof("Remove strimzi resources")
	listOpts := []client.ListOption{
		client.InNamespace(utils.GetDefaultNamespace()),
	}
	kafkaUserList := &kafkav1beta2.KafkaUserList{}
	log.Infof("Delete kafkaUsers")
	if err := r.c.List(ctx, kafkaUserList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}
	for idx := range kafkaUserList.Items {
		log.Infof("Delete kafka user %v", kafkaUserList.Items[idx].Name)
		if err := r.c.Delete(ctx, &kafkaUserList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	kafkaTopicList := &kafkav1beta2.KafkaTopicList{}
	log.Infof("Delete kafkaTopics")

	if err := r.c.List(ctx, kafkaTopicList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}
	for idx := range kafkaTopicList.Items {
		log.Infof("Delete kafka topic %v", kafkaTopicList.Items[idx].Name)
		if err := r.c.Delete(ctx, &kafkaTopicList.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	// Wait kafkatopic is removed
	if err := r.c.List(ctx, kafkaTopicList, listOpts...); err != nil {
		return ctrl.Result{}, err
	}

	if len(kafkaTopicList.Items) != 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	kafka := &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      strimziTransport.kafkaClusterName,
			Namespace: utils.GetDefaultNamespace(),
		},
	}
	log.Infof("Delete kafka cluster %v", kafka.Name)

	if err := r.c.Delete(ctx, kafka); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	log.Infof("kafka cluster deleted")

	// Delete kafka pvc
	kafkaPvc := &corev1.PersistentVolumeClaimList{}
	pvcListOpts := []client.ListOption{
		client.MatchingLabels(map[string]string{
			"strimzi.io/cluster": KafkaClusterName,
		}),
	}
	if err := r.c.List(ctx, kafkaPvc, pvcListOpts...); err != nil {
		return ctrl.Result{}, err
	}
	for idx := range kafkaPvc.Items {
		log.Infof("Delete kafka pvc %v", kafkaPvc.Items[idx].Name)
		if err := r.c.Delete(ctx, &kafkaPvc.Items[idx]); err != nil && !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	kafkaSub := &subv1alpha1.Subscription{}
	err := r.c.Get(ctx, types.NamespacedName{
		Namespace: utils.GetDefaultNamespace(),
		Name:      strimziTransport.subName,
	}, kafkaSub)
	if err != nil {
		log.Errorf("Failed to get strimzi subscription, err:%v", err)
		if errors.IsNotFound(err) {
			isResourceRemoved = true
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
		log.Infof("Delete kafka csv %v", kafkaCsv.Name)
		if err := r.c.Delete(ctx, kafkaCsv); err != nil {
			return ctrl.Result{}, err
		}
		log.Infof("kafka csv deleted")
	}

	if err := r.c.Delete(ctx, kafkaSub); err != nil && !errors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	log.Infof("kafka subscription deleted")
	isResourceRemoved = true
	return ctrl.Result{}, nil
}
