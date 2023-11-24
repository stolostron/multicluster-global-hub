package protocol

import (
	"context"
	"fmt"
	"time"

	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	"github.com/go-logr/logr"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
)

const (
	// kafka - common
	DefaultKakaName = "kafka"
	// kafka storage
	DefaultKafkaDefaultStorageSize       = "10Gi"
	Replicas1                      int32 = 1
	Replicas2                      int32 = 2

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
)

var (
	KafkaStorageIdentifier  int32 = 0
	KafkaStorageDeleteClaim       = false
	KafkaVersion                  = "3.5.0"
)

// install the strimzi kafka cluster by operator
type StrimziKafka struct {
	log           logr.Logger
	ctx           context.Context
	name          string
	namespace     string
	runtimeClient client.Client

	// subscription properties
	community            bool
	subName              string
	subConfig            *subv1alpha1.SubscriptionConfig
	subChannel           string
	subCatalogSourceName string
	subPackageName       string

	// installer
	installer *types.NamespacedName

	// kafka cluster
	kafkaStorageSize *string
}

type KafkaOption func(*StrimziKafka)

func NewStrimziKafka(opts ...KafkaOption) *StrimziKafka {
	k := &StrimziKafka{
		name:      DefaultKakaName,
		namespace: constants.GHDefaultNamespace,

		installer: &types.NamespacedName{
			Name:      "multiclusterglobalhub",
			Namespace: constants.GHDefaultNamespace,
		},

		community:            false,
		subName:              DefaultKafkaSubName,
		subChannel:           DefaultAMQChannel,
		subPackageName:       DefaultAMQPackageName,
		subCatalogSourceName: DefaultCatalogSourceName,
	}
	// apply options
	for _, opt := range opts {
		opt(k)
	}

	if k.community {
		k.subChannel = CommunityChannel
		k.subPackageName = CommunityPackageName
		k.subCatalogSourceName = CommunityCatalogSourceName
	}
	return k
}

func WithName(name string) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.name = name
	}
}

func WithNamespace(namespace string) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.namespace = namespace
	}
}

func WithCommunity(val bool) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.community = val
	}
}

func WithClient(c client.Client) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.runtimeClient = c
	}
}

func WithSubName(name string) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.subName = name
	}
}

func WithSubConfig(config *subv1alpha1.SubscriptionConfig) KafkaOption {
	return func(sk *StrimziKafka) {
		sk.subConfig = config
	}
}

// initialize the kafka cluster, return nil if the instance is launched successfully!
func (k *StrimziKafka) Initialize(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
	k.log.Info("reconcile global hub kafka subscription")
	err := k.reconcileSubscription()
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

// waits for kafka cluster to be ready and returns nil if kafka cluster ready
func (k *StrimziKafka) kafkaClusterReady() error {
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

func (k *StrimziKafka) createKafkaCluster(mgh *globalhubv1alpha4.MulticlusterGlobalHub) error {
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

func (k *StrimziKafka) newKafkaCluster(mgh *globalhubv1alpha4.MulticlusterGlobalHub) *kafkav1beta2.Kafka {
	kafkaSpecKafkaStorageVolumesElem := kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
		Id:          &KafkaStorageIdentifier,
		Size:        config.GetKafkaStorageSize(mgh),
		Type:        kafkav1beta2.KafkaSpecKafkaStorageVolumesElemTypePersistentClaim,
		DeleteClaim: &KafkaStorageDeleteClaim,
	}
	kafkaSpecZookeeperStorage := kafkav1beta2.KafkaSpecZookeeperStorage{
		Type:        kafkav1beta2.KafkaSpecZookeeperStorageTypePersistentClaim,
		Size:        config.GetKafkaStorageSize(mgh),
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
func (k *StrimziKafka) reconcileSubscription() error {
	// get subscription
	existingSub := &subv1alpha1.Subscription{}
	err := k.runtimeClient.Get(k.ctx, types.NamespacedName{
		Name:      k.subName,
		Namespace: k.namespace,
	}, existingSub)
	if err != nil && !errors.IsNotFound(err) {
		return err
	}

	expectedSub := k.newSubscription()
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
func (k *StrimziKafka) newSubscription() *subv1alpha1.Subscription {
	labels := map[string]string{
		"installer.name":                 k.installer.Name,
		"installer.namespace":            k.installer.Namespace,
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
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
			Config:                 k.subConfig,
		},
	}
	return sub
}
