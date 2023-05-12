package consumer

import (
	"strings"
	"testing"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGenerateConsumer(t *testing.T) {
	mockKafkaCluster, err := kafka.NewMockCluster(1)
	if err != nil {
		t.Errorf("failed to init mock kafka cluster - %v", err)
	}
	transportConfig := &transport.TransportConfig{
		TransportType: "kafka",
		KafkaConfig: &transport.KafkaConfig{
			BootstrapServer: mockKafkaCluster.BootstrapServers(),
			EnableTLS:       false,
			ConsumerConfig: &transport.KafkaConsumerConfig{
				ConsumerID:    "test-consumer",
				ConsumerTopic: "test-topic",
			},
		},
	}
	_, err = NewGenericConsumer(transportConfig)
	if err != nil && !strings.Contains(err.Error(), "client has run out of available brokers") {
		t.Errorf("failed to generate consumer - %v", err)
	}
}
