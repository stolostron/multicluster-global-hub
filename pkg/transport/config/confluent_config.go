package config

import (
	"io/ioutil"
	"log"
	"strings"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
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

	if kafkaConfig.EnableTLS && Validate(kafkaConfig.CaCertPath) {
		if err := kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return nil, err
		}
		if err := kafkaConfigMap.SetKey("ssl.ca.location", kafkaConfig.CaCertPath); err != nil {
			return nil, err
		}

		if Validate(kafkaConfig.ClientCertPath) && Validate(kafkaConfig.ClientKeyPath) {
			_ = kafkaConfigMap.SetKey("ssl.certificate.location", kafkaConfig.ClientCertPath)
			_ = kafkaConfigMap.SetKey("ssl.key.location", kafkaConfig.ClientKeyPath)
		}
	}
	return kafkaConfigMap, nil
}

// Validate checks if the file exists and the content is not empty
func Validate(filePath string) bool {
	if len(filePath) == 0 {
		return false
	}
	content, err := ioutil.ReadFile(filePath) // #nosec G304
	if err != nil {
		log.Printf("failed to read file %s - %v", filePath, err)
		return false
	}
	trimmedContent := strings.TrimSpace(string(content))
	return len(trimmedContent) > 0
}
