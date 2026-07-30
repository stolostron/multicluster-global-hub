package utils

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

var sensitiveKafkaConfigKeys = map[string]struct{}{
	"bootstrap.servers":   {},
	"sasl.username":       {},
	"sasl.password":       {},
	"ssl.ca.pem":          {},
	"ssl.certificate.pem": {},
	"ssl.key.pem":         {},
	"ssl.key.password":    {},
}

// FilterSensitiveKafkaConfig filters out sensitive data from Kafka ConfigMap for safe logging.
// It replaces broker endpoints, credentials, and certificate/key values with "[REDACTED]".
func FilterSensitiveKafkaConfig(configMap *kafka.ConfigMap) map[string]interface{} {
	safeConfig := make(map[string]interface{})
	for key, value := range *configMap {
		if _, sensitive := sensitiveKafkaConfigKeys[key]; sensitive {
			safeConfig[key] = "[REDACTED]"
		} else {
			safeConfig[key] = value
		}
	}
	return safeConfig
}
