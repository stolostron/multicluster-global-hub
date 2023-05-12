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
	TopicDefault   = "status"
	GroupIDDefault = "my-group2"
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

	ctx, cancel := context.WithCancel(context.TODO())
	go func() {
		// this method calls the methods handler on each stage: setup, consume and cleanup
		if err := consumerGroup.Consume(ctx, []string{TopicDefault}, &consumerGroupHandler{}); err != nil {
			log.Printf("Error from consumer: %v", err)
		}
	}()

	// waiting for the end of all messages received or an OS signal
	sig := <-signals
	log.Printf("Got signal: %v\n", sig)
	cancel()

	log.Printf("Closing consumer group")
	err = consumerGroup.Close()
	if err != nil {
		log.Printf("Error closing the Sarama consumer: %v", err)
		os.Exit(1)
	}
	log.Printf("Consumer closed")
}

// struct defining the handler for the consuming Sarama method
type consumerGroupHandler struct{}

func (cgh *consumerGroupHandler) Setup(sarama.ConsumerGroupSession) error {
	log.Printf("Consumer group handler setup\n")
	return nil
}

func (cgh *consumerGroupHandler) Cleanup(sarama.ConsumerGroupSession) error {
	log.Printf("Consumer group handler cleanup\n")
	return nil
}

func (cgh *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession,
	claim sarama.ConsumerGroupClaim,
) error {
	for message := range claim.Messages() {
		log.Printf("Message received: value=%s, partition=%d, offset=%d", string(message.Value),
			message.Partition, message.Offset)
		session.MarkMessage(message, "") // must mark the message after consume, otherwise the auto commit will not work
	}
	return nil
}
