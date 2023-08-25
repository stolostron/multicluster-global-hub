// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package kafka

import (
	kafkav1beta2 "github.com/RedHatInsights/strimzi-client-go/apis/kafka.strimzi.io/v1beta2"
	subv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	globalhubv1alpha4 "github.com/stolostron/multicluster-global-hub/operator/apis/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

var (
	SubscriptionName = "strimzi-kafka-operator"

	// prod postgres variables
	channel                = "stable"
	installPlanApproval    = subv1alpha1.ApprovalAutomatic
	packageName            = "amq-streams"
	catalogSourceName      = "redhat-operators"
	catalogSourceNamespace = "openshift-marketplace"

	// community postgres variables
	communityChannel           = "stable"
	communityPackageName       = "strimzi-kafka-operator"
	communityCatalogSourceName = "community-operators"

	// default names
	KafkaClusterName     = "strimzi-kafka-cluster"
	KafkaSpecTopicName   = "spec"
	KafkaStatusTopicName = "status"
	KafkaEventTopicName  = "event"
	KafkaUserName        = "global-hub-kafka-user"

	// kafka version
	kafkaVersion = "3.4.0"

	// kafka storage
	kafkaStorageSize              = "10Gi"
	kafkaStorageIndentifier int32 = 0
	kafkaStorageDeleteClaim       = false

	replicas1 int32 = 1
	replicas2 int32 = 2
)

type KafkaConnection struct {
	BootstrapServer string
	CACert          string
	ClientCert      string
	ClientKey       string
}

// NewSubscription returns an CrunchyPostgres subscription with desired default values
func NewSubscription(m *globalhubv1alpha4.MulticlusterGlobalHub, c *subv1alpha1.SubscriptionConfig,
	community bool) *subv1alpha1.Subscription {
	chName, pkgName, catSourceName := channel, packageName, catalogSourceName
	if community {
		chName = communityChannel
		pkgName = communityPackageName
		catSourceName = communityCatalogSourceName
	}
	labels := map[string]string{
		"installer.name":                 m.GetName(),
		"installer.namespace":            m.GetNamespace(),
		constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
	}
	sub := &subv1alpha1.Subscription{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
			Kind:       subv1alpha1.SubscriptionKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      SubscriptionName,
			Namespace: constants.GHDefaultNamespace,
			Labels:    labels,
		},
		Spec: &subv1alpha1.SubscriptionSpec{
			Channel:                chName,
			InstallPlanApproval:    installPlanApproval,
			Package:                pkgName,
			CatalogSource:          catSourceName,
			CatalogSourceNamespace: catalogSourceNamespace,
			Config:                 c,
		},
	}

	return sub
}

// RenderSubscription returns a subscription by modifying the spec of an existing subscription based on overrides
func RenderSubscription(existingSubscription *subv1alpha1.Subscription, config *subv1alpha1.SubscriptionConfig,
	community bool) *subv1alpha1.Subscription {
	copy := existingSubscription.DeepCopy()
	copy.ManagedFields = nil
	copy.TypeMeta = metav1.TypeMeta{
		APIVersion: subv1alpha1.SubscriptionCRDAPIVersion,
		Kind:       subv1alpha1.SubscriptionKind,
	}

	chName, pkgName, catSourceName := channel, packageName, catalogSourceName
	if community {
		chName = communityChannel
		pkgName = communityPackageName
		catSourceName = communityCatalogSourceName
	}

	copy.Spec = &subv1alpha1.SubscriptionSpec{
		Channel:                chName,
		InstallPlanApproval:    installPlanApproval,
		Package:                pkgName,
		CatalogSource:          catSourceName,
		CatalogSourceNamespace: catalogSourceNamespace,
		Config:                 config,
	}

	// if updating channel must remove startingCSV
	if copy.Spec.Channel != existingSubscription.Spec.Channel {
		copy.Spec.StartingCSV = ""
	}

	return copy
}

// NewKafka creates kafka operand
func NewKafka() *kafkav1beta2.Kafka {
	return &kafkav1beta2.Kafka{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KafkaClusterName,
			Namespace: constants.GHDefaultNamespace,
		},
		Spec: &kafkav1beta2.KafkaSpec{
			Kafka: kafkav1beta2.KafkaSpecKafka{
				Config: &apiextensions.JSON{Raw: []byte(`{
"default.replication.factor": 3,
"inter.broker.protocol.version": "3.4",
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
				Replicas: 3,
				Storage: kafkav1beta2.KafkaSpecKafkaStorage{
					Type: kafkav1beta2.KafkaSpecKafkaStorageTypeJbod,
					Volumes: []kafkav1beta2.KafkaSpecKafkaStorageVolumesElem{
						{
							Id:          &kafkaStorageIndentifier,
							Size:        &kafkaStorageSize,
							Type:        kafkav1beta2.KafkaSpecKafkaStorageVolumesElemTypePersistentClaim,
							DeleteClaim: &kafkaStorageDeleteClaim,
						},
					},
				},
				Version: &kafkaVersion,
			},
			Zookeeper: kafkav1beta2.KafkaSpecZookeeper{
				Replicas: 3,
				Storage: kafkav1beta2.KafkaSpecZookeeperStorage{
					Type:        kafkav1beta2.KafkaSpecZookeeperStorageTypePersistentClaim,
					Size:        &kafkaStorageSize,
					DeleteClaim: &kafkaStorageDeleteClaim,
				},
			},
			EntityOperator: &kafkav1beta2.KafkaSpecEntityOperator{
				TopicOperator: &kafkav1beta2.KafkaSpecEntityOperatorTopicOperator{},
				UserOperator:  &kafkav1beta2.KafkaSpecEntityOperatorUserOperator{},
			},
		},
	}
}

// NewKafkaTopic creates kafkatopic operand
// In global hub context, spec/status/event topics are created
func NewKafkaTopic(topicName, namespace string) *kafkav1beta2.KafkaTopic {
	return &kafkav1beta2.KafkaTopic{
		ObjectMeta: metav1.ObjectMeta{
			Name:      topicName,
			Namespace: namespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the topic will not be ready
				"strimzi.io/cluster": KafkaClusterName,
			},
		},
		Spec: &kafkav1beta2.KafkaTopicSpec{
			Partitions: &replicas1,
			Replicas:   &replicas2,
			Config: &apiextensions.JSON{Raw: []byte(`{
				"cleanup.policy": "compact"
			}`)},
		},
	}
}

// NewKafkaUser creates kafkauser operand
// TODO: @clyang82 create a user for each managed hub
func NewKafkaUser(username, namespace string) *kafkav1beta2.KafkaUser {
	return &kafkav1beta2.KafkaUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      username,
			Namespace: namespace,
			Labels: map[string]string{
				// It is important to set the cluster label otherwise the user will not be ready
				"strimzi.io/cluster": KafkaClusterName,
			},
		},
		Spec: &kafkav1beta2.KafkaUserSpec{
			Authentication: &kafkav1beta2.KafkaUserSpecAuthentication{
				Type: kafkav1beta2.KafkaUserSpecAuthenticationTypeTls,
			},
		},
	}
}
