package utils

import (
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// FilterSensitiveKafkaConfig filters out sensitive data from Kafka ConfigMap for safe logging.
// It replaces SSL certificate and key values with "[REDACTED]" to prevent exposure of
// sensitive credentials in logs.
func FilterSensitiveKafkaConfig(configMap *kafka.ConfigMap) map[string]interface{} {
	safeConfig := make(map[string]interface{})
	for key, value := range *configMap {
		// Exclude sensitive SSL/TLS certificate and key data
		if key == "ssl.ca.pem" || key == "ssl.certificate.pem" || key == "ssl.key.pem" {
			safeConfig[key] = "[REDACTED]"
		} else {
			safeConfig[key] = value
		}
	}
	return safeConfig
}
