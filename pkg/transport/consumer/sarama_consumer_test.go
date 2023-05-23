package consumer

import (
	"context"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var testMsg = sarama.StringEncoder("Foo")

func TestConsumerGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Given
	fetchResponse1 := &sarama.FetchResponse{Version: 4}
	fetchResponse1.AddRecord("my_topic", 0, nil, testMsg, 1)

	fetchResponse2 := &sarama.FetchResponse{Version: 4}
	fetchResponse2.AddRecord("my_topic", 0, nil, testMsg, 2)

	broker0 := sarama.NewMockBroker(t, 0)

	broker0.SetHandlerByMap(map[string]sarama.MockResponse{
		"MetadataRequest": sarama.NewMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": sarama.NewMockOffsetResponse(t).
			SetOffset("my_topic", 0, sarama.OffsetNewest, 1234).
			SetOffset("my_topic", 0, sarama.OffsetOldest, 0),
		"FetchRequest": sarama.NewMockSequence(fetchResponse1, fetchResponse2),
	})

	kafkaConfig := &transport.KafkaConfig{
		BootstrapServer: broker0.Addr(),
		EnableTLS:       false,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID:    "test-consumer",
			ConsumerTopic: "my_topic",
		},
	}

	consumer, err := NewSaramaConsumer(ctx, kafkaConfig)
	if err != nil {
		t.Errorf("failed to init sarama consumer - %v", err)
	}

	if consumer == nil {
		t.Errorf("failed to init sarama consumer - %v", err)
	}

	go func() {
		_ = consumer.Start(ctx)
	}()

	// msg := <-consumer.MessageChan()
	// fmt.Printf("msg: %v\n", msg)
	cancel()
}
