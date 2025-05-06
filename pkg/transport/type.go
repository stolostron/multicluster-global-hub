package transport

import (
	"time"
)

const (
	Broadcast      = "broadcast" // Broadcast can be used as destination when a bundle should be broadcasted.
	ChunkSizeKey   = "extsize"   // ChunkSizeKey is the key used for total bundle size header.
	ChunkOffsetKey = "extoffset" // ChunkOffsetKey is the key used for message fragment offset header.
)

// indicate the transport type, only support kafka or go chan
type TransportType string

const (
	// transportType values
	Kafka TransportType = "kafka"
	Chan  TransportType = "chan"
	Rest  TransportType = "rest"
)

// transport protocol
// indicate which kind of transport protocol, only support
type TransportProtocol int

const (
	// the kafka cluster is created by the strimzi operator, which is provisioned by global hub operator
	StrimziTransporter TransportProtocol = iota
	// the kafka cluster is created by customer, and the transport secret will be shared between clusters
	SecretTransporter
)

type TransportInternalConfig struct {
	TransportType     string
	CommitterInterval time.Duration
	// EnableDatabaseOffset affects only the manager, deciding if consumption starts from a database-stored offset
	EnableDatabaseOffset bool
	ConsumerGroupId      string
	// set the kafka credentail in the transport controller
	KafkaCredential   *KafkaConfig
	RestfulCredential *RestfulConfig
	Extends           map[string]interface{}
	FailureThreshold  int
}

// KafkaInternalConfig specifics the configuration for the global hub manager, agent, or even inventory
type KafkaInternalConfig struct {
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

// topics
type ClusterTopic struct {
	SpecTopic   string
	StatusTopic string
}

type EventPosition struct {
	Topic     string `json:"-"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
	// define the kafka cluster identiy:
	// 1. built in kafka, use the kafka cluster id
	// 2. byo kafka, use the kafka bootstrapserver as the identity
	OwnerIdentity string `json:"ownerIdentity"`
}
