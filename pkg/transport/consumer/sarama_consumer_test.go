package consumer

import (
	"context"
	"testing"

	"github.com/Shopify/sarama"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/test/pkg/kafka"
)

func TestConsumerGroup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Given
	responses := []string{"Foo", "Bar"}
	kafkaCluster := kafka.MockSaramaCluster(t, responses)
	defer kafkaCluster.Close()

	kafkaConfig := &transport.KafkaConfig{
		BootstrapServer: kafkaCluster.Addr(),
		EnableTLS:       false,
		ConsumerConfig: &transport.KafkaConsumerConfig{
			ConsumerID: "test-consumer",
		},
	}

	// consumer, err := NewSaramaConsumer(ctx, kafkaConfig)
	// if err != nil {
	// 	t.Errorf("failed to init sarama consumer - %v", err)
	// }
	// go func() {
	// 	_ = consumer.Start(ctx)
	// }()

	// msg := <-consumer.MessageChan()
	// fmt.Printf("msg: %v\n", msg)
	// time.Sleep(5 * time.Second)
	// cancel()

	saramaConfig, err := config.GetSaramaConfig(kafkaConfig)
	if err != nil {
		t.Fatal(err)
	}
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	group, err := sarama.NewConsumerGroup([]string{kafkaConfig.BootstrapServer}, "my-group", saramaConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = group.Close() }()

	// used to commit offset asynchronously
	processedChan := make(chan *sarama.ConsumerMessage)
	messageChan := make(chan *sarama.ConsumerMessage)

	// test handler
	handler := &consumeGroupHandler{
		log:           ctrl.Log.WithName("sarama-consumer").WithName("handler"),
		messageChan:   messageChan,
		processedChan: processedChan,
	}

	go func() {
		topics := []string{"my-topic"}
		for {
			if err := group.Consume(ctx, topics, handler); err != nil {
				t.Error(err)
			}
			// check if context was cancelled, signaling that the consumer should stop
			if ctx.Err() != nil {
				return
			}
		}
	}()

	msg := <-messageChan
	got1 := string(msg.Value)
	if got1 != responses[0] {
		t.Errorf("got %s, want %s", got1, responses[0])
	}

	msg = <-messageChan
	got2 := string(msg.Value)
	if got2 != responses[1] {
		t.Errorf("got %s, want %s", got2, responses[1])
	}

	cancel()
}
