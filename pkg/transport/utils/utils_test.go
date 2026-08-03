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
				"group.id":          "test-group",
				"security.protocol": "SSL",
			},
			expected: map[string]interface{}{
				"group.id":          "test-group",
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
				"sasl.username":       "user",
				"sasl.password":       "secret",
			},
			expected: map[string]interface{}{
				"ssl.ca.pem":          "[REDACTED]",
				"ssl.certificate.pem": "[REDACTED]",
				"ssl.key.pem":         "[REDACTED]",
				"bootstrap.servers":   "[REDACTED]",
				"sasl.username":       "[REDACTED]",
				"sasl.password":       "[REDACTED]",
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
				"bootstrap.servers": "[REDACTED]",
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
			assertFilteredKafkaConfig(t, result, tt.expected)
		})
	}
}

func assertFilteredKafkaConfig(t *testing.T, got, want map[string]interface{}) {
	t.Helper()

	for key, wantVal := range want {
		gotVal, ok := got[key]
		if !ok {
			t.Errorf("missing key %q in filtered config", key)
			continue
		}
		if reflect.DeepEqual(gotVal, wantVal) {
			continue
		}
		if _, sensitive := sensitiveKafkaConfigKeys[key]; sensitive {
			t.Errorf("key %q: expected redaction marker, got unexpected value", key)
			continue
		}
		t.Errorf("key %q: got %v, want %v", key, gotVal, wantVal)
	}

	for key := range got {
		if _, ok := want[key]; !ok {
			t.Errorf("unexpected key %q in filtered config", key)
		}
	}
}
