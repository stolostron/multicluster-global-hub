package transport

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	KafkaCredential   *KafkaConnCredential
	RestfulCredential *RestfulConnCredentail
	Extends           map[string]interface{}
}

type RestfulConnCredentail struct {
	Host                 string `yaml:"host"`
	CommonConnCredential        // Embed common fields
}

// YamlMarshal marshal the connection credential object, rawCert specifies whether to keep the cert in the data directly
func (k *RestfulConnCredentail) YamlMarshal(rawCert bool) ([]byte, error) {
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

func (k *RestfulConnCredentail) DeepCopy() *RestfulConnCredentail {
	return &RestfulConnCredentail{
		Host:                 k.Host,
		CommonConnCredential: *k.CommonConnCredential.DeepCopy(),
	}
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

type CommonConnCredential struct {
	CACert           string `yaml:"ca.crt,omitempty"`
	ClientCert       string `yaml:"client.crt,omitempty"`
	ClientKey        string `yaml:"client.key,omitempty"`
	CASecretName     string `yaml:"ca.secret,omitempty"`
	ClientSecretName string `yaml:"client.secret,omitempty"`
}

func (c *CommonConnCredential) DeepCopy() *CommonConnCredential {
	return &CommonConnCredential{
		CACert:           c.CACert,
		ClientCert:       c.ClientCert,
		ClientKey:        c.ClientKey,
		CASecretName:     c.CASecretName,
		ClientSecretName: c.ClientSecretName,
	}
}

func (conn *CommonConnCredential) ParseCommonCredentailFromSecret(namespace string, c client.Client,
) error {
	// decode the ca cert, client key and cert
	if conn.CACert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.CACert)
		if err != nil {
			return err
		}
		conn.CACert = string(bytes)
	}
	if conn.ClientCert != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientCert)
		if err != nil {
			return err
		}
		conn.ClientCert = string(bytes)
	}
	if conn.ClientKey != "" {
		bytes, err := base64.StdEncoding.DecodeString(conn.ClientKey)
		if err != nil {
			return err
		}
		conn.ClientKey = string(bytes)
	}

	// load the ca cert from secret
	if conn.CASecretName != "" {
		caSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      conn.CASecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(caSecret), caSecret); err != nil {
			return err
		}
		conn.CACert = string(caSecret.Data["ca.crt"])
	}
	// load the client key and cert from secret
	if conn.ClientSecretName != "" {
		clientSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace,
				Name:      conn.ClientSecretName,
			},
		}
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(clientSecret), clientSecret); err != nil {
			return fmt.Errorf("failed to get the client cert: %w", err)
		}
		conn.ClientCert = string(clientSecret.Data["tls.crt"])
		conn.ClientKey = string(clientSecret.Data["tls.key"])
		if conn.ClientCert == "" || conn.ClientKey == "" {
			return fmt.Errorf("the client cert or key must not be empty: %s", conn.ClientSecretName)
		}
	}
	return nil
}

// KafkaConnCredential is used to connect the transporter instance. The field is persisted to secret
// need to be encode with base64.StdEncoding.EncodeToString
type KafkaConnCredential struct {
	BootstrapServer      string `yaml:"bootstrap.server"`
	StatusTopic          string `yaml:"topic.status,omitempty"`
	SpecTopic            string `yaml:"topic.spec,omitempty"`
	ClusterID            string `yaml:"cluster.id,omitempty"`
	CommonConnCredential        // Embed common fields
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
		BootstrapServer:      k.BootstrapServer,
		StatusTopic:          k.StatusTopic,
		SpecTopic:            k.SpecTopic,
		ClusterID:            k.ClusterID,
		CommonConnCredential: *k.CommonConnCredential.DeepCopy(),
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
