package consumer_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/samples/consumer"
)

type handler struct {
	*testing.T
	ctx    context.Context
	cancel context.CancelFunc
}

func (h *handler) Setup(s sarama.ConsumerGroupSession) error {
	// period commit offset
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		for {
			select {
			case <-ticker.C:
				fmt.Println(">>> commit offset")
				s.Commit()
			case <-h.ctx.Done():
				fmt.Println(">>> last commit offset")
				s.Commit()
				ticker.Stop()
				return
			}
		}
	}()
	return nil
}

func (h *handler) Cleanup(s sarama.ConsumerGroupSession) error {
	return nil
}

func (h *handler) ConsumeClaim(sess sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for msg := range claim.Messages() {
		// mark offset, manually commit offset: https://github.com/Shopify/sarama/issues/2441
		fmt.Println(">>> mark offset")
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

	group, err := sarama.NewConsumerGroup([]string{server}, "my-kafka-group", config)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = group.Close() }()

	ctx, cancel := context.WithCancel(context.Background())

	h := &handler{t, ctx, cancel}
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
