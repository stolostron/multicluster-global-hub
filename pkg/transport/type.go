package transport

import (
	"time"
)

const (
	// Broadcast can be used as destination when a bundle should be broadcasted.
	Broadcast = "broadcast"
	// ChunkSizeKey is the key used for total bundle size header.
	ChunkSizeKey = "size"
	// ChunkOffsetKey is the key used for message fragment offset header.
	ChunkOffsetKey = "offset"

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
	Key         string `json:"key"`
	Destination string `json:"destination"`
	MsgType     string `json:"msgType"`
	Payload     []byte `json:"payload"`
}

// ConnCredential is used to connect the transporter instance
type ConnCredential struct {
	BootstrapServer string
	CACert          string
	ClientCert      string
	ClientKey       string
}
