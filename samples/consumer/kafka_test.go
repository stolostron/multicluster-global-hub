package consumer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Shopify/sarama"
)

// handler represents a Sarama consumer of consumer group
type handler struct {
	*testing.T
	ctx    context.Context
	cancel context.CancelFunc
}

// Setup is run at the beginning of a new session, before ConsumeClaim
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

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *handler) Cleanup(s sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (h *handler) ConsumeClaim(sess sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for {
		select {
		case msg := <-claim.Messages():
			// mark offset, manually commit offset: https://github.com/Shopify/sarama/issues/2441
			fmt.Println("+++ mark offset")
			sess.MarkMessage(msg, "")
			h.Logf("consumed msg %d - %d: %s", msg.Partition, msg.Offset, string(msg.Value))
			h.cancel()
		// Should return when `session.Context()` is done.
		// If not, will raise `ErrRebalanceInProgress` or `read tcp <ip>:<port>: i/o timeout` when kafka rebalance. see:
		// https://github.com/Shopify/sarama/issues/1192
		case <-sess.Context().Done():
			return nil

		case <-h.ctx.Done():
			fmt.Println("--- stop the receiver")
			return nil
		}
	}
}

func TestKafkaConsumer(t *testing.T) {
	server, config, err := GetSaramaConfig()
	if err != nil {
		t.Fatalf("get sarama config: %v", err)
	}

	group, err := sarama.NewConsumerGroup([]string{server}, "my-kafka-group-0", config)
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
