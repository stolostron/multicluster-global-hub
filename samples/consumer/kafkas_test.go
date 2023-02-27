package consumer_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/samples/consumer"
)

type handler struct {
	*testing.T
	cancel context.CancelFunc
}

func (h *handler) Setup(s sarama.ConsumerGroupSession) error   { return nil }
func (h *handler) Cleanup(s sarama.ConsumerGroupSession) error { return nil }
func (h *handler) ConsumeClaim(sess sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		sess.MarkMessage(msg, "")
		h.Logf("consumed msg %d - %d: %s", msg.Partition, msg.Offset, string(msg.Value))
		h.cancel()
		break
	}
	return nil
}

func TestKafkaConsumer(t *testing.T) {
	server, config, err := consumer.GetSaramaConfig()
	if err != nil {
		t.Fatalf("get sarama config: %v", err)
	}
	// manually commit offset seems not work: https://github.com/Shopify/sarama/issues/2441
	config.Consumer.Offsets.AutoCommit.Enable = true

	group, err := sarama.NewConsumerGroup([]string{server}, "my-kafka-group", config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = group.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	h := &handler{t, cancel}
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		topics := []string{"status"}
		if err := group.Consume(ctx, topics, h); err != nil {
			t.Error(err)
		}
		wg.Done()
	}()
	wg.Wait()
}
