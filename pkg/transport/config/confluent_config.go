package config

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func GetConfluentConfigMap(kafkaConfig *transport.KafkaConfig) (*kafka.ConfigMap, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       kafkaConfig.BootstrapServer,
		"socket.keepalive.enable": "true",
		"auto.offset.reset":       "earliest", // consumer
		"enable.auto.commit":      "false",    // consumer
		"acks":                    "1",        // producer
		"retries":                 "0",        // producer
		// silence spontaneous disconnection logs, kafka recovers by itself.
		"log.connection.close": "false",
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
