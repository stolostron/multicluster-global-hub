package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

var (
	groupId = "test-group-id"
	topic   = "event"
)

func main() {
	bootstrapServer, saramaConfig, err := config.GetSaramaConfig()
	if err != nil {
		log.Fatalf("failed to get sarama config: %v", err)
	}
	// if set this to false, it will consume message from beginning when restart the client
	saramaConfig.Consumer.Offsets.AutoCommit.Enable = false
	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest

	receiver, err := kafka_sarama.NewConsumer([]string{bootstrapServer}, saramaConfig, groupId, topic)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	defer receiver.Close(context.Background())

	c, err := cloudevents.NewClient(receiver)
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
	fmt.Printf("%s \n", event.Data())
}
