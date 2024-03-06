package transport

import (
	"time"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

const (
	GenericSpecTopic   = "spec"
	GenericStatusTopic = "status"
	GenericEventTopic  = "event"

	Broadcast      = "broadcast" // Broadcast can be used as destination when a bundle should be broadcasted.
	ChunkSizeKey   = "extsize"   // ChunkSizeKey is the key used for total bundle size header.
	ChunkOffsetKey = "extoffset" // ChunkOffsetKey is the key used for message fragment offset header.

	// Deprecated
	// CompressionType is the key used for compression type header.
	CompressionType = "content-encoding"
	// FragmentationTimestamp is the key used for bundle fragmentation time header.
	FragmentationTimestamp = "fragmentation-timestamp"
	DestinationKey         = "destination"
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
	MessageCompressionType string
	CommitterInterval      time.Duration
	KafkaConfig            *KafkaConfig
	Extends                map[string]interface{}
}

// Kafka Config
type KafkaConfig struct {
	ClusterIdentity string
	BootstrapServer string
	CaCertPath      string
	ClientCertPath  string
	ClientKeyPath   string
	EnableTLS       bool
	Topics          *ClusterTopic
	ProducerConfig  *KafkaProducerConfig
	ConsumerConfig  *KafkaConsumerConfig
}

type KafkaProducerConfig struct {
	ProducerID         string
	MessageSizeLimitKB int
}

type KafkaConsumerConfig struct {
	ConsumerID string
}

// transport protocol
// indicate which kind of transport protocol, only support
type TransportProtocol int

const (
	// the kafka cluster is created by the strimzi operator, which is provisioned by global hub operator
	StrimziTransporter TransportProtocol = iota
	// the kafka cluster is created by customer, and the transport secret will be shared between clusters
	SecretTransporter
)

// topics
type ClusterTopic struct {
	SpecTopic   string
	StatusTopic string
	EventTopic  string
}

// Message abstracts a message object to be used by different transport components.
type Message struct {
	Key          string                `json:"key"`
	Destination  string                `json:"destination"`
	MsgType      string                `json:"msgType"`
	Payload      []byte                `json:"payload"`
	BundleStatus metadata.BundleStatus // the manager to mark the processing status of the bundle
}

// ConnCredential is used to connect the transporter instance
type ConnCredential struct {
	Identity        string
	BootstrapServer string
	CACert          string
	ClientCert      string
	ClientKey       string
}
