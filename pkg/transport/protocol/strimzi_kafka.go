package protocol

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
)

const (
	// kafka storage
	DefaultKafkaDefaultStorageSize = "10Gi"

	// subscription - common
	DefaultKafkaSubName           = "strimzi-kafka-operator"
	DefaultInstallPlanApproval    = subv1alpha1.ApprovalAutomatic
	DefaultCatalogSourceNamespace = "openshift-marketplace"

	// subscription - production
	DefaultAMQChannel        = "amq-streams-2.5.x"
	DefaultAMQPackageName    = "amq-streams"
	DefaultCatalogSourceName = "redhat-operators"

	// subscription - community
	CommunityChannel           = "strimzi-0.36.x"
	CommunityPackageName       = "strimzi-kafka-operator"
	CommunityCatalogSourceName = "community-operators"

	// users
	DefaultGlobalHubKafkaUser = "global-hub-kafka-user"

	// topic names
	GlobalHubTopicIdentity = "*"
	SpecTopicTemplate      = "GlobalHub.Spec.%s"
	StatusTopicTemplate    = "GlobalHub.Status.%s"
	EventTopicTemplate     = "GlobalHub.Event.%s"
)

var (
	KafkaStorageIdentifier  int32 = 0
	KafkaStorageDeleteClaim       = false
	KafkaVersion                  = "3.5.0"
	DefaultPartition        int32 = 1
	DefaultReplicas         int32 = 2
)

// install the strimzi kafka cluster by operator
type strimziKafka struct {
	log           logr.Logger
	ctx           context.Context
	name          string
	namespace     string
	runtimeClient client.Client

	// subscription properties
	subName              string
	subCommunity         bool
	subChannel           string
	subCatalogSourceName string
	subPackageName       string

	// global hub config
	mgh *globalhubv1alpha4.MulticlusterGlobalHub
}

type KafkaOption func(*strimziKafka)

func NewStrimziKafka(opts ...KafkaOption) (*strimziKafka, error) {
	k := &strimziKafka{
		log:       ctrl.Log.WithName("strimzi-kafka-transport"),
		ctx:       context.TODO(),
		name:      config.GetKafkaClusterName(),
		namespace: constants.GHDefaultNamespace,

		subName:              DefaultKafkaSubName,
		subCommunity:         false,
		subChannel:           DefaultAMQChannel,
		subPackageName:       DefaultAMQPackageName,
		subCatalogSourceName: DefaultCatalogSourceName,
	}
	// apply options
	for _, opt := range opts {
		opt(k)
	}

	if k.subCommunity {
		k.subChannel = CommunityChannel
		k.subPackageName = CommunityPackageName
		k.subCatalogSourceName = CommunityCatalogSourceName
	}
	err := k.initialize(k.mgh)
	return k, err
}

func WithNamespacedName(name types.NamespacedName) KafkaOption {
	return func(sk *strimziKafka) {
		sk.name = name.Name
		sk.namespace = name.Namespace
	}
}

func WithClient(c client.Client) KafkaOption {
	return func(sk *strimziKafka) {
		sk.runtimeClient = c
	}
}

func WithContext(ctx context.Context) KafkaOption {
	return func(sk *strimziKafka) {
		sk.ctx = ctx
	}
}

func WithCommunity(val bool) KafkaOption {
	return func(sk *strimziKafka) {
		sk.subCommunity = val
	}
}

func WithRuntimeClient(c client.Client) KafkaOption {
	return func(sk *strimziKafka) {
		sk.runtimeClient = c
	}
}

func WithSubName(name string) KafkaOption {
	return func(sk *strimziKafka) {
		sk.subName = name
	}
}

func WithGlobalHub(mgh *globalhubv1alpha4.MulticlusterGlobalHub) KafkaOption {
	return func(sk *strimziKafka) {
		sk.mgh = mgh
	}
}

// initialize the kafka cluster, return nil if the instance is launched successfully!
func (k *strimziKafka) initialize(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	k.log.Info("reconcile global hub kafka subscription")
	err := k.ensureSubscription(mgh)
	if err != nil {
		return err
	}

	k.log.Info("reconcile global hub kafka instance")
	err = wait.PollUntilContextTimeout(k.ctx, 2*time.Second, 30*time.Second, true,
		func(ctx context.Context) (bool, error) {
			err = k.createKafkaCluster(mgh)
			if err != nil {
				k.log.Info("the kafka instance is not created, retrying...", "message", err.Error())
				return false, nil
			}
			return true, nil
		})
	if err != nil {
		return err
	}

	k.log.Info("waiting the kafka cluster instance to be ready...")
	err = wait.PollUntilContextTimeout(k.ctx, 5*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			err := k.kafkaClusterReady()
			if err != nil {
				k.log.Info("the kafka is not ready, waiting", "message", err.Error())
				return false, nil
			}
			return true, nil
		})
	return err
}

func (k *strimziKafka) CreateUser(username string) error {
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      username,
		Namespace: k.namespace,
	}, kafkaUser)
	if err != nil && errors.IsNotFound(err) {
		return k.runtimeClient.Create(k.ctx, k.newKafkaUser(username))
	}
	return err
}

func (k *strimziKafka) DeleteUser(topicName string) error {
	kafkaUser := &kafkav1beta2.KafkaUser{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      topicName,
		Namespace: k.namespace,
	}, kafkaUser)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return k.runtimeClient.Delete(k.ctx, kafkaUser)
}

func (k *strimziKafka) GetTopicNames(clusterIdentity string) *transport.ClusterTopic {
	return &transport.ClusterTopic{
		SpecTopic:   fmt.Sprintf(SpecTopicTemplate, clusterIdentity),
		StatusTopic: fmt.Sprintf(StatusTopicTemplate, clusterIdentity),
		EventTopic:  fmt.Sprintf(EventTopicTemplate, clusterIdentity),
	}
}

func (k *strimziKafka) GetUserName(clusterIdentity string) string {
	return fmt.Sprintf("%s-kafka-user", clusterIdentity)
}

func (k *strimziKafka) CreateTopic(topicNames []string) error {
	for _, topicName := range topicNames {
		kafkaTopic := &kafkav1beta2.KafkaTopic{}
		err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
			Name:      topicName,
			Namespace: k.namespace,
		}, kafkaTopic)
		if err != nil && errors.IsNotFound(err) {
			if e := k.runtimeClient.Create(k.ctx, k.newKafkaTopic(topicName)); e != nil {
				return e
			}
		}
	}
	return nil
}

func (k *strimziKafka) DeleteTopic(topicNames []string) error {
	for _, topicName := range topicNames {
		kafkaTopic := &kafkav1beta2.KafkaTopic{}
		err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
			Name:      topicName,
			Namespace: k.namespace,
		}, kafkaTopic)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return err
		}
		err = k.runtimeClient.Delete(k.ctx, kafkaTopic)
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *strimziKafka) GetConnCredential(username string) (*transport.ConnCredential, error) {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      k.name,
		Namespace: k.namespace,
	}, kafkaCluster)
	if err != nil {
		return nil, err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return nil, fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	kafkaUserSecret := &corev1.Secret{}
	err = k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      username,
		Namespace: k.namespace,
	}, kafkaUserSecret)
	if err != nil {
		return nil, err
	}

	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			return &transport.ConnCredential{
				BootstrapServer: *kafkaCluster.Status.Listeners[1].BootstrapServers,
				CACert: base64.StdEncoding.EncodeToString(
					[]byte(kafkaCluster.Status.Listeners[1].Certificates[0])),
				ClientCert: base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.crt"]),
				ClientKey:  base64.StdEncoding.EncodeToString(kafkaUserSecret.Data["user.key"]),
			}, nil
		}
	}

	return nil, fmt.Errorf("kafka user %s/%s is not ready", k.namespace, username)
}

func (k *strimziKafka) newKafkaTopic(topicName string) *kafkav1beta2.KafkaTopic {
	return &kafkav1beta2.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      topicName,
			Namespace: k.namespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the topic will not be ready
				"strimzi.io/cluster":             k.name,
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubAddonOwnerLabelVal,
			},
		},
		Spec: &kafkav1beta2.KafkaTopicSpec{
			Partitions: &DefaultPartition,
			Replicas:   &DefaultReplicas,
			Config: &apiextensions.JSON{Raw: []byte(`{
				"cleanup.policy": "compact"
			}`)},
		},
	}
}

func (k *strimziKafka) newKafkaUser(username string) *kafkav1beta2.KafkaUser {
	return &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      username,
			Namespace: k.namespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the user will not be ready
				"strimzi.io/cluster":             k.name,
				constants.GlobalHubOwnerLabelKey: constants.GlobalHubAddonOwnerLabelVal,
			},
		},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authentication: &kafkav1beta2.KafkaUserSpecAuthentication{
				Type: kafkav1beta2.KafkaUserSpecAuthenticationTypeTls,
			},
		},
	}
}

// waits for kafka cluster to be ready and returns nil if kafka cluster ready
func (k *strimziKafka) kafkaClusterReady() error {
	kafkaCluster := &kafkav1beta2.Kafka{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      k.name,
		Namespace: k.namespace,
	}, kafkaCluster)
	if err != nil {
		return err
	}

	if kafkaCluster.Status == nil || kafkaCluster.Status.Conditions == nil {
		return fmt.Errorf("kafka cluster %s has no status conditions", kafkaCluster.Name)
	}

	for _, condition := range kafkaCluster.Status.Conditions {
		if *condition.Type == "Ready" && *condition.Status == "True" {
			return nil
		}
	}
	return fmt.Errorf("kafka cluster %s is not ready", kafkaCluster.Name)
}

func (k *strimziKafka) createKafkaCluster(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	existingKafka := &kafkav1beta2.Kafka{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      k.name,
		Namespace: k.namespace,
	}, existingKafka)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	if err == nil {
		return nil
	}

	// not found
	return k.runtimeClient.Create(k.ctx, k.newKafkaCluster(mgh))
}

func (k *strimziKafka) newKafkaCluster(mgh *globalhubv1alpha4.MulticlusterGlobalHub) *kafkav1beta2.Kafka {
	storageSize := config.GetKafkaStorageSize(mgh)
	kafkaSpecKafkaStorageVolumesElem := kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
		Id:          &KafkaStorageIdentifier,
		Size:        &storageSize,
		Type:        kafkav1beta2.KafkaSpecKafkaStorageVolumesElemTypePersistentClaim,
		DeleteClaim: &KafkaStorageDeleteClaim,
	}
	kafkaSpecZookeeperStorage := kafkav1beta2.KafkaSpecZookeeperStorage{
		Type:        kafkav1beta2.KafkaSpecZookeeperStorageTypePersistentClaim,
		Size:        &storageSize,
		DeleteClaim: &KafkaStorageDeleteClaim,
	}

	if mgh.Spec.DataLayer.StorageClass != "" {
		kafkaSpecKafkaStorageVolumesElem.Class = &mgh.Spec.DataLayer.StorageClass
		kafkaSpecZookeeperStorage.Class = &mgh.Spec.DataLayer.StorageClass
	}

	return &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.name,
			Namespace: k.namespace,
		},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Config: &apiextensions.JSON{Raw: []byte(`{
"default.replication.factor": 3,
"inter.broker.protocol.version": "3.5",
"min.insync.replicas": 2,
"offsets.topic.replication.factor": 3,
"transaction.state.log.min.isr": 2,
"transaction.state.log.replication.factor": 3
}`)},
				Listeners: []kafkav1beta2.KafkaSpecKafkaListenersElem{
					{
						Name: "plain",
						Port: 9092,
						Tls:  false,
						Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeInternal,
					},
					{
						Name: "tls",
						Port: 9093,
						Tls:  true,
						Type: kafkav1beta2.KafkaSpecKafkaListenersElemTypeRoute,
						Authentication: &kafkav1beta2.KafkaSpecKafkaListenersElemAuthentication{
							Type: kafkav1beta2.KafkaSpecKafkaListenersElemAuthenticationTypeTls,
						},
					},
				},
				Resources: &kafkav1beta2.KafkaSpecKafkaResources{
					Requests: &apiextensions.JSON{Raw: []byte(`{
"memory": "1Gi",
"cpu": "100m"
}`)},
					Limits: &apiextensions.JSON{Raw: []byte(`{
"memory": "4Gi"
}`)},
				},
				Replicas: 3,
				Storage: kafkav1beta2.KafkaSpecKafkaStorage{
					Type: kafkav1beta2.KafkaSpecKafkaStorageTypeJbod,
					Volumes: []kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
						kafkaSpecKafkaStorageVolumesElem,
					},
				},
				Version: &KafkaVersion,
			},
			Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
				Replicas: 3,
				Storage:  kafkaSpecZookeeperStorage,
				Resources: &kafkav1beta2.KafkaSpecZookeeperResources{
					Requests: &apiextensions.JSON{Raw: []byte(`{
"memory": "500Mi",
"cpu": "20m"
}`)},
					Limits: &apiextensions.JSON{Raw: []byte(`{
"memory": "3Gi"
}`)},
				},
			},
			EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
				TopicOperator: &kafkav1beta2.KafkaSpecEntityOperatorTopicOperator{},
				UserOperator:  &kafkav1beta2.KafkaSpecEntityOperatorUserOperator{},
			},
		},
	}
}

// create/ update the kafka subscription
func (k *strimziKafka) ensureSubscription(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	// get subscription
	existingSub := &subv1alpha1.Subscription{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      k.subName,
		Namespace: k.namespace,
	}, existingSub)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	expectedSub := k.newSubscription(mgh)
	if errors.IsNotFound(err) {
		return k.runtimeClient.Create(k.ctx, expectedSub)
	} else {
		startingCSV := expectedSub.Spec.StartingCSV
		// if updating channel must remove startingCSV
		if existingSub.Spec.Channel != expectedSub.Spec.Channel {
			startingCSV = ""
		}
		if !equality.Semantic.DeepEqual(existingSub.Spec, expectedSub.Spec) {
			existingSub.Spec = expectedSub.Spec
		}
		existingSub.Spec.StartingCSV = startingCSV
		return k.runtimeClient.Update(k.ctx, existingSub)
	}
}

// newSubscription returns an CrunchyPostgres subscription with desired default values
func (k *strimziKafka) newSubscription(mgh *globalhubv1alpha4.MulticlusterGlobalHub) *subv1alpha1.Subscription {
	labels := map[string]string{
		"installer.name":                 mgh.Name,
		"installer.namespace":            mgh.Namespace,
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
	}
	// Generate sub config from mgh CR
	subConfig := &subv1alpha1.SubscriptionConfig{
		NodeSelector: mgh.Spec.NodeSelector,
		Tolerations:  mgh.Spec.Tolerations,
	}

	sub := &subv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
			Kind:       subv1alpha1.SubscriptionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      k.subName,
			Namespace: k.namespace,
			Labels:    labels,
		},
		Spec: &subv1alpha1.SubscriptionSpec{
			Channel:                k.subChannel,
			InstallPlanApproval:    DefaultInstallPlanApproval,
			Package:                k.subPackageName,
			CatalogSource:          k.subCatalogSourceName,
			CatalogSourceNamespace: DefaultCatalogSourceNamespace,
			Config:                 subConfig,
		},
	}
	return sub
}
