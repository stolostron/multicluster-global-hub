package config

import (
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func GetConfluentConfigMap(kafkaConfig *transport.KafkaConfig, producer bool) (*kafka.ConfigMap, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       kafkaConfig.BootstrapServer,
		"socket.keepalive.enable": "true",
		// silence spontaneous disconnection logs, kafka recovers by itself.
		"log.connection.close": "true",
	}
	if producer {
		_ = kafkaConfigMap.SetKey("acks", "1")
		_ = kafkaConfigMap.SetKey("retries", "0")
	} else {
		_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")
		_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
		_ = kafkaConfigMap.SetKey("group.id", kafkaConfig.ConsumerConfig.ConsumerID)
	}

	if kafkaConfig.EnableTLS && utils.Validate(kafkaConfig.CaCertPath) {
		if err := setCertificate(kafkaConfig.CaCertPath); err != nil {
			return nil, err
		}
		if err := kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return nil, err
		}
		if err := kafkaConfigMap.SetKey("ssl.ca.location", kafkaConfig.CaCertPath); err != nil {
			return nil, err
		}
		if utils.Validate(kafkaConfig.ClientCertPath) && utils.Validate(kafkaConfig.ClientKeyPath) {
			_ = kafkaConfigMap.SetKey("ssl.certificate.location", kafkaConfig.ClientCertPath)
			_ = kafkaConfigMap.SetKey("ssl.key.location", kafkaConfig.ClientKeyPath)
		}
	}
	return kafkaConfigMap, nil
}

// registers the ca in root certification authority.
func setCertificate(caCertPath string) error {
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
		return errors.New("kafka-certificate-manager: failed to append certificate")
	}
	return nil
}
