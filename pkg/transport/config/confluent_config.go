package config

import (
	"crypto/x509"
	"errors"
	"os"
	"path/filepath"

	kafkav2 "github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

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
	_ = kafkaConfigMap.SetKey("go.produce.channel.size", 1000)
	_ = kafkaConfigMap.SetKey("acks", "1")
	_ = kafkaConfigMap.SetKey("retries", "3")
	_ = kafkaConfigMap.SetKey("go.events.channel.size", 1000)
}

func SetConsumerConfig(kafkaConfigMap *kafkav2.ConfigMap, groupId string) {
	_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")
	_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	_ = kafkaConfigMap.SetKey("group.id", groupId)
	_ = kafkaConfigMap.SetKey("go.events.channel.size", 1000)
}

func SetTLSByLocation(kafkaConfigMap *kafkav2.ConfigMap, caCertPath, certPath, keyPath string) error {
	_, validCA := utils.Validate(caCertPath)
	if !validCA {
		return errors.New("invalid ca certificate")
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

func SetTLSByRawData(kafkaConfigMap *kafkav2.ConfigMap, caCrt, clientCrt, clientKey string) error {
	_ = kafkaConfigMap.SetKey("security.protocol", "ssl")
	if err := kafkaConfigMap.SetKey("ssl.ca.pem", caCrt); err != nil {
		return err
	}
	if err := kafkaConfigMap.SetKey("ssl.certificate.pem", clientCrt); err != nil {
		return err
	}
	if err := kafkaConfigMap.SetKey("ssl.key.pem", clientKey); err != nil {
		return err
	}
	return nil
}

// https://github.com/confluentinc/librdkafka/blob/master/CONFIGURATION.md
func GetConfluentConfigMap(kafkaConfig *transport.KafkaConfig, producer bool) (*kafkav2.ConfigMap, error) {
	kafkaConfigMap := GetBasicConfigMap()
	_ = kafkaConfigMap.SetKey("bootstrap.servers", kafkaConfig.ConnCredential.BootstrapServer)
	if producer {
		SetProducerConfig(kafkaConfigMap)
	} else {
		SetConsumerConfig(kafkaConfigMap, kafkaConfig.ConsumerConfig.ConsumerID)
	}
	if !kafkaConfig.EnableTLS {
		return kafkaConfigMap, nil
	}

	err := SetTLSByRawData(kafkaConfigMap,
		kafkaConfig.ConnCredential.CACert,
		kafkaConfig.ConnCredential.ClientCert,
		kafkaConfig.ConnCredential.ClientKey)
	if err != nil {
		return nil, err
	}
	return kafkaConfigMap, nil
}
