package config

import (
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
		kafkaConfigMap.SetKey("acks", "1")
		kafkaConfigMap.SetKey("retries", "0")
	} else {
		kafkaConfigMap.SetKey("enable.auto.commit", "false")
		kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	}

	if kafkaConfig.EnableTLS && utils.Validate(kafkaConfig.CaCertPath) {
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
