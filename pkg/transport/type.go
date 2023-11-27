package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DestinationHub is the key used for destination-hub name header.
	DestinationHub = "destination-hub"
	// CompressionType is the key used for compression type header.
	CompressionType = "content-encoding"
	// Size is the key used for total bundle size header.
	Size = "size"
	// Offset is the key used for message fragment offset header.
	Offset = "offset"
	// FragmentationTimestamp is the key used for bundle fragmentation time header.
	FragmentationTimestamp = "fragmentation-timestamp"
	// Broadcast can be used as destination when a bundle should be broadcasted.
	Broadcast = ""
)

// indicate the transport type, only support kafka or go chan
type TransportType string

const (
	// transportType values
	Kafka TransportType = "kafka"
	Chan  TransportType = "chan"
)

type TransportConfig struct {
	TransportType          string
	TransportFormat        string
	MessageCompressionType string
	CommitterInterval      time.Duration
	KafkaConfig            *KafkaConfig
	Extends                map[string]interface{}
}

// Kafka Config
type KafkaConfig struct {
	BootstrapServer string
	CaCertPath      string
	ClientCertPath  string
	ClientKeyPath   string
	EnableTLS       bool
	ProducerConfig  *KafkaProducerConfig
	ConsumerConfig  *KafkaConsumerConfig
}

type KafkaProducerConfig struct {
	ProducerID         string
	ProducerTopic      string
	MessageSizeLimitKB int
}

type KafkaConsumerConfig struct {
	ConsumerID    string
	ConsumerTopic string
}

// transport protocol
// indicate which kind of transport protocol, only support
type TransportProtocol int

const (
	// the kafka cluster is created by the strimzi operator, which is provisioned by global hub operator
	InternalTransport TransportProtocol = iota
	// the kafka cluster is created by customer, and the transport secret will be shared between clusters
	ExternalTransport
)

func DetectTransportProtocol(ctx context.Context, runtimeClient client.Client) (TransportProtocol, error) {
	// get the transport secret
	kafkaSecret := &corev1.Secret{}
	err := runtimeClient.Get(ctx, types.NamespacedName{
		Name:      constants.GHTransportSecretName,
		Namespace: config.GetDefaultNamespace(),
	}, kafkaSecret)
	if err == nil {
		return ExternalTransport, nil
	}
	if err != nil && !errors.IsNotFound(err) {
		return ExternalTransport, err
	}

	// the transport secret is not found
	return InternalTransport, nil
}

// topics
const (
	GlobalHubTopicIdentity        = "*"
	managedHubSpecTopicTemplate   = "GlobalHub.Spec.%s"
	managedHubStatusTopicTemplate = "GlobalHub.Status.%s"
	managedHubEventTopicTemplate  = "GlobalHub.Event.%s"
)

func GetTopics(clusterIdentity string) *ClusterTopic {
	return &ClusterTopic{
		SpecTopic:   fmt.Sprintf(managedHubSpecTopicTemplate, clusterIdentity),
		StatusTopic: fmt.Sprintf(managedHubStatusTopicTemplate, clusterIdentity),
		EventTopic:  fmt.Sprintf(managedHubEventTopicTemplate, clusterIdentity),
	}
}
