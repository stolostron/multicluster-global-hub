// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package protocol

import (
	"context"
	"embed"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
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
	trans       *strimziTransporter
	kafkaStatus KafkaStatus
}

func IsResourceRemoved() bool {
	log.Infof("KafkaController resource removed: %v", isResourceRemoved)
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
	defer func() {
		err = config.UpdateMGHComponent(ctx, r.c, r.getKafkaComponentStatus(err, r.kafkaStatus), false)
		if err != nil {
			log.Errorf("failed to update mgh status, err:%v", err)
		}
	}()
	needRequeue, err := r.trans.EnsureKafka()
	if err != nil {
		return ctrl.Result{}, err
	}
	if needRequeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.kafkaStatus, err = r.trans.kafkaClusterReady()
	if err != nil {
		return ctrl.Result{}, err
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
	// update the transport connection, if conn is nil, requeue to let it ready
	conn, err := getManagerTransportConn(r.trans)
	if err != nil {
		return ctrl.Result{}, err
	}
	if conn == nil {
		log.Infow("waiting the kafka cluster credential to be ready...", "message", "kafka cluster credential is not ready")
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Create/update transport-config secret for manager
	if err := CreateManagerTransportSecret(ctx, mgh, conn, r.c); err != nil {
		log.Errorf("failed to create manager transport-config secret: %v", err)
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

func StartKafkaController(ctx context.Context, mgr ctrl.Manager, transporter transport.Transporter) error {
	if startedKafkaController {
		return nil
	}
	log.Info("start kafka controller")
	r := &KafkaController{
		c:     mgr.GetClient(),
		trans: transporter.(*strimziTransporter),
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

	// consumer group id
	conn.ConsumerGroupID = config.GetConsumerGroupID(trans.mgh.Spec.DataLayerSpec.Kafka.ConsumerGroupPrefix,
		constants.CloudEventGlobalHubClusterName)
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

	if !config.IsTransportConfigReady(r.trans.ctx, r.trans.mgh.Namespace, r.c) {
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

// CreateManagerTransportSecret creates or updates the transport-config secret for manager
func CreateManagerTransportSecret(ctx context.Context, mgh *v1alpha4.MulticlusterGlobalHub,
	kafkaConfig *transport.KafkaConfig, c client.Client,
) error {
	secretData := make(map[string][]byte)
	// Build kafka config yaml
	if kafkaConfig != nil {
		kafkaConfigYaml, err := kafkaConfig.YamlMarshal(true)
		if err != nil {
			return fmt.Errorf("failed to marshal kafka config: %w", err)
		}
		secretData["kafka.yaml"] = kafkaConfigYaml
	}

	// Add inventory config if enabled
	if config.WithInventory(mgh) {
		inventoryConn, err := certificates.GetInventoryCredential(c)
		if err != nil {
			log.Warnf("failed to get inventory credential: %v", err)
		} else {
			inventoryConfigYaml, err := inventoryConn.YamlMarshal(true)
			if err != nil {
				log.Warnf("failed to marshal inventory config: %v", err)
			} else {
				secretData["rest.yaml"] = inventoryConfigYaml
			}
		}
	}

	if len(secretData) == 0 {
		return fmt.Errorf("no kafka config or inventory config to create transport-config secret")
	}

	// Create or update the secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.GHTransportConfigSecret,
			Namespace: mgh.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.Type = corev1.SecretTypeOpaque
		secret.Data = secretData
		secret.Labels = map[string]string{
			"name": "multicluster-global-hub-manager",
		}
		// Set MGH as the owner of this secret
		return controllerutil.SetControllerReference(mgh, secret, c.Scheme())
	})
	return err
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
			Name:      r.trans.kafkaClusterName,
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
		Name:      r.trans.subName,
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
