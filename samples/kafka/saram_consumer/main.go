package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shopify/sarama"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	TopicDefault        = "status"
	GroupIDDefault      = "my-group2"
	MessageCountDefault = 10
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGKILL)

	bootstrapSever, saramaConfig, err := config.GetSaramaConfig()
	if err != nil {
		log.Panicf("Error getting the consumer config: %v", err)
		os.Exit(1)
	}
	// if set this to false, it will consume message from beginning when restart the client
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	consumerGroup, err := sarama.NewConsumerGroup([]string{string(bootstrapSever)}, GroupIDDefault, saramaConfig)
	if err != nil {
		log.Printf("Error creating the Sarama consumer: %v", err)
		os.Exit(1)
	}

	cgh := &consumerGroupHandler{
		toReceive: MessageCountDefault,
		end:       make(chan int, 1),
	}
	ctx := context.Background()
	go func() {
		for {
			// this method calls the methods handler on each stage: setup, consume and cleanup
			consumerGroup.Consume(ctx, []string{TopicDefault}, cgh)
		}
	}()

	// waiting for the end of all messages received or an OS signal
	select {
	case <-cgh.end:
		log.Printf("Finished to receive %d messages\n", MessageCountDefault)
	case sig := <-signals:
		log.Printf("Got signal: %v\n", sig)
	}

	err = consumerGroup.Close()
	if err != nil {
		log.Printf("Error closing the Sarama consumer: %v", err)
		os.Exit(1)
	}
	log.Printf("Consumer closed")
}

// struct defining the handler for the consuming Sarama method
type consumerGroupHandler struct {
	toReceive int64
	end       chan int
}

func (cgh *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	log.Printf("Consumer group handler setup\n")
	return nil
}

func (cgh *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Printf("Consumer group handler cleanup\n")
	return nil
}

func (cgh *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for message := range claim.Messages() {
		log.Printf("Message received: value=%s, partition=%d, offset=%d", string(message.Value), message.Partition, message.Offset)
		session.MarkMessage(message, "") // must mark the message after consume, otherwise the auto commit will not work
		if cgh.toReceive--; cgh.toReceive == 0 {
			cgh.end <- 1
			break
		}
	}
	return nil
}
