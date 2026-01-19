package config

import (
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	// Bytes => 10 MiB * 1024 * 1024 => Set it into the maximum size of a single message
	// to avoid the message being truncated mid-transmission.
	MaxSizeToChunk = 10 * 1024 * 1024

	// message.max.bytes default value is 1000000
	MaxSizeToSend = 10 * 1000 * 1000
	// fetch.message.max.bytes default value is 1048576
	MaxSizeToFetch = 10 * 1024 * 1024
)

var (
	log                = logger.DefaultZapLogger()
	kafkaOwnerIdentity string
)

func SetKafkaOwnerIdentity(identity string) {
	kafkaOwnerIdentity = identity
}

func GetKafkaOwnerIdentity() string {
	return kafkaOwnerIdentity
}

func GetBasicConfigMap() *kafkav2.ConfigMap {
	return &kafkav2.ConfigMap{
		"socket.keepalive.enable": "true",
		// silence spontaneous disconnection logs, kafka recovers by itself.
		"log.connection.close": "false",
		// https://github.com/confluentinc/librdkafka/issues/4349
		"ssl.endpoint.identification.algorithm": "none",
	}
}

func SetProducerConfig(kafkaConfigMap *kafkav2.ConfigMap) {
	_ = kafkaConfigMap.SetKey("message.max.bytes", MaxSizeToSend)
	_ = kafkaConfigMap.SetKey("acks", "1")
	_ = kafkaConfigMap.SetKey("retries", "1")
	// Enable snappy compression to reduce message size in broker memory (page cache)
	_ = kafkaConfigMap.SetKey("compression.type", "snappy")
}

func SetConsumerConfig(kafkaConfigMap *kafkav2.ConfigMap, groupId string, topicMetadataRefreshInterval int) {
	_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")
	_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	_ = kafkaConfigMap.SetKey("group.id", groupId)
	_ = kafkaConfigMap.SetKey("max.partition.fetch.bytes", MaxSizeToFetch)
	_ = kafkaConfigMap.SetKey("fetch.message.max.bytes", MaxSizeToFetch)
	if topicMetadataRefreshInterval > 0 {
		_ = kafkaConfigMap.SetKey("metadata.max.age.ms", fmt.Sprintf("%d", topicMetadataRefreshInterval))
		_ = kafkaConfigMap.SetKey("topic.metadata.refresh.interval.ms", fmt.Sprintf("%d", topicMetadataRefreshInterval))
	}
}

func SetTLSByLocation(kafkaConfigMap *kafkav2.ConfigMap, caCertPath, certPath, keyPath string) error {
	_, validCA := utils.Validate(caCertPath)
	if !validCA {
		return errors.New("invalid ca certificate for tls")
	}
	_, validCert := utils.Validate(certPath)
	_, validKey := utils.Validate(keyPath)
	if !validCert || !validKey {
		return errors.New("invalid client key or cert")
	}

	if err := kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
		return err
	}
	if err := kafkaConfigMap.SetKey("ssl.ca.location", caCertPath); err != nil {
		return err
	}

	// set ca
	certBytes, err := os.ReadFile(filepath.Clean(caCertPath))
	if err != nil {
		return err
	}
	// Get the SystemCertPool, continue with an empty pool on error
	rootCertAuth, _ := x509.SystemCertPool()
	if rootCertAuth == nil {
		rootCertAuth = x509.NewCertPool()
	}

	// Append our cert to the system pool
	if ok := rootCertAuth.AppendCertsFromPEM(certBytes); !ok {
		return errors.New("failed to append ca certificate")
	}

	// set client certificate
	_ = kafkaConfigMap.SetKey("ssl.certificate.location", certPath)
	_ = kafkaConfigMap.SetKey("ssl.key.location", keyPath)
	return nil
}

// https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md
func GetConfluentConfigMap(kafkaConfig *transport.KafkaInternalConfig, producer bool) (*kafkav2.ConfigMap, error) {
	kafkaConfigMap := GetBasicConfigMap()
	_ = kafkaConfigMap.SetKey("bootstrap.servers", kafkaConfig.BootstrapServer)
	if producer {
		SetProducerConfig(kafkaConfigMap)
	} else {
		SetConsumerConfig(kafkaConfigMap, kafkaConfig.ConsumerConfig.ConsumerID, 0)
	}
	if !kafkaConfig.EnableTLS {
		return kafkaConfigMap, nil
	}
	err := SetTLSByLocation(kafkaConfigMap, kafkaConfig.CaCertPath, kafkaConfig.ClientCertPath, kafkaConfig.ClientKeyPath)
	if err != nil {
		return nil, err
	}
	return kafkaConfigMap, nil
}

// GetConfluentConfigMapByConfig tries to connect the kafka with transport secret(ca.key, client.crt, client.key)
func GetConfluentConfigMapByConfig(transportConfig *corev1.Secret, c client.Client, consumerGroupID string) (
	*kafkav2.ConfigMap, error,
) {
	conn, err := utils.GetKafkaCredentialBySecret(transportConfig, c)
	if err != nil {
		return nil, err
	}
	return GetConfluentConfigMapByKafkaCredential(conn, consumerGroupID, 0)
}

func GetConfluentConfigMapByKafkaCredential(conn *transport.KafkaConfig,
	consumerGroupID string, topicMetadataRefreshInterval int) (
	*kafkav2.ConfigMap, error,
) {
	kafkaConfigMap := GetBasicConfigMap()
	if consumerGroupID != "" {
		SetConsumerConfig(kafkaConfigMap, consumerGroupID, topicMetadataRefreshInterval)
	} else {
		SetProducerConfig(kafkaConfigMap)
	}
	_ = kafkaConfigMap.SetKey("bootstrap.servers", conn.BootstrapServer)
	// if the certs is invalid
	if conn.CACert == "" || conn.ClientCert == "" || conn.ClientKey == "" {
		log.Warn("Connect to Kafka without SSL")
		return kafkaConfigMap, nil
	}

	_ = kafkaConfigMap.SetKey("security.protocol", "ssl")
	if err := kafkaConfigMap.SetKey("ssl.ca.pem", conn.CACert); err != nil {
		return nil, err
	}

	if err := kafkaConfigMap.SetKey("ssl.certificate.pem", conn.ClientCert); err != nil {
		return nil, err
	}

	if err := kafkaConfigMap.SetKey("ssl.key.pem", conn.ClientKey); err != nil {
		return nil, err
	}
	return kafkaConfigMap, nil
}

// GetKafkaUserName gives a kafkaUser name based on the cluster name, it's also the CN of the certificate
func GetKafkaUserName(clusterName string) string {
	return fmt.Sprintf("%s-kafka-user", clusterName)
}
