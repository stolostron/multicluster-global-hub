package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/IBM/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"

	"github.com/stolostron/multicluster-global-hub/samples/config"
)

var groupId = "test-group-id"

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]

	bootstrapServer, saramaConfig, err := config.GetSaramaConfigByTranportConfig("open-cluster-management")
	if err != nil {
		log.Fatalf("failed to get sarama config: %v", err)
	}
	// if set this to false, it will consume message from beginning when restart the client,
	// otherwise it will consume message from the last committed offset.
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	receiver, err := kafka_sarama.NewConsumer([]string{bootstrapServer}, saramaConfig, groupId, topic)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	defer func() {
		if err := receiver.Close(context.Background()); err != nil {
			log.Printf("failed to close receiver: %v", err)
		}
	}()

	c, err := cloudevents.NewClient(receiver, client.WithPollGoroutines(1))
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	log.Printf("will listen consuming topic: %s\n", topic)
	err = c.StartReceiver(context.Background(), receive)
	if err != nil {
		log.Fatalf("failed to start receiver: %s", err)
	} else {
		log.Printf("receiver stopped\n")
	}
}

func receive(ctx context.Context, event cloudevents.Event) {
	fmt.Println(event)
}
