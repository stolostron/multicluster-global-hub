package utils

import (
	"reflect"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

func TestFilterSensitiveKafkaConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    kafka.ConfigMap
		expected map[string]interface{}
	}{
		{
			name: "No sensitive keys",
			input: kafka.ConfigMap{
				"bootstrap.servers": "localhost:9092",
				"security.protocol": "SSL",
			},
			expected: map[string]interface{}{
				"bootstrap.servers": "localhost:9092",
				"security.protocol": "SSL",
			},
		},
		{
			name: "All sensitive keys present",
			input: kafka.ConfigMap{
				"ssl.ca.pem":          "ca-data",
				"ssl.certificate.pem": "cert-data",
				"ssl.key.pem":         "key-data",
				"bootstrap.servers":   "localhost:9092",
			},
			expected: map[string]interface{}{
				"ssl.ca.pem":          "[REDACTED]",
				"ssl.certificate.pem": "[REDACTED]",
				"ssl.key.pem":         "[REDACTED]",
				"bootstrap.servers":   "localhost:9092",
			},
		},
		{
			name: "Some sensitive keys present",
			input: kafka.ConfigMap{
				"ssl.ca.pem":        "ca-data",
				"bootstrap.servers": "localhost:9092",
			},
			expected: map[string]interface{}{
				"ssl.ca.pem":        "[REDACTED]",
				"bootstrap.servers": "localhost:9092",
			},
		},
		{
			name:     "Empty config",
			input:    kafka.ConfigMap{},
			expected: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterSensitiveKafkaConfig(&tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("FilterSensitiveKafkaConfig() = %v, want %v", result, tt.expected)
			}
		})
	}
}
