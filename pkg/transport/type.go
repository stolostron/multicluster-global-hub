package transport

import (
	"time"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
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
	// Multiple indciates the agent maybe report the event to multiple target, like kafka and rest API
	Multiple TransportType = "multiple"
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

type TransportConfig struct {
	TransportType     string
	CommitterInterval time.Duration
	// IsManager specifies the send/receive topics from specTopic and statusTopic
	// For example, SpecTopic sends and statusTopic receives on the manager; the agent is the opposite
	IsManager bool
	// EnableDatabaseOffset affects only the manager, deciding if consumption starts from a database-stored offset
	EnableDatabaseOffset bool
	ConsumerGroupId      string
	// set the kafka credentail in the transport controller
	KafkaCredential     *KafkaConnCredential
	InventoryCredentail *InventoryConnCredentail
	Extends             map[string]interface{}
}

type InventoryConnCredentail struct {
	Host       string `yaml:"host"`
	CACert     string `yaml:"ca.crt,omitempty"`
	ClientCert string `yaml:"client.crt,omitempty"`
	ClientKey  string `yaml:"client.key,omitempty"`
}

func (k *InventoryConnCredentail) YamlMarshal() ([]byte, error) {
	bytes, err := yaml.Marshal(k)
	return bytes, err
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

// topics
type ClusterTopic struct {
	SpecTopic   string
	StatusTopic string
}

// KafkaConnCredential is used to connect the transporter instance. The field is persisted to secret
// need to be encode with base64.StdEncoding.EncodeToString
type KafkaConnCredential struct {
	BootstrapServer string `yaml:"bootstrap.server"`
	StatusTopic     string `yaml:"topic.status,omitempty"`
	SpecTopic       string `yaml:"topic.spec,omitempty"`
	ClusterID       string `yaml:"cluster.id,omitempty"`
	// the following fields are only for the manager, and the agent of byo/standalone kafka
	CACert     string `yaml:"ca.crt,omitempty"`
	ClientCert string `yaml:"client.crt,omitempty"`
	ClientKey  string `yaml:"client.key,omitempty"`
	// the following fields are only for the agent of built-in kafka
	CASecretName     string `yaml:"ca.secret,omitempty"`
	ClientSecretName string `yaml:"client.secret,omitempty"`
}

// YamlMarshal marshal the connection credential object, rawCert specifies whether to keep the cert in the data directly
func (k *KafkaConnCredential) YamlMarshal(rawCert bool) ([]byte, error) {
	copy := k.DeepCopy()
	if rawCert {
		copy.CASecretName = ""
		copy.ClientSecretName = ""
	} else {
		copy.CACert = ""
		copy.ClientCert = ""
		copy.ClientKey = ""
	}
	bytes, err := yaml.Marshal(copy)
	return bytes, err
}

// DeepCopy creates a deep copy of KafkaConnCredential
func (k *KafkaConnCredential) DeepCopy() *KafkaConnCredential {
	return &KafkaConnCredential{
		BootstrapServer:  k.BootstrapServer,
		StatusTopic:      k.StatusTopic,
		SpecTopic:        k.SpecTopic,
		ClusterID:        k.ClusterID,
		CACert:           k.CACert,
		ClientCert:       k.ClientCert,
		ClientKey:        k.ClientKey,
		CASecretName:     k.CASecretName,
		ClientSecretName: k.ClientSecretName,
	}
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
