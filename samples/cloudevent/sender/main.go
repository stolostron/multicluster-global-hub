package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/IBM/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	count = 10
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]

	// bootstrapServer, saramaConfig, err := config.GetSaramaConfig()
	bootstrapServer, saramaConfig, err := config.GetSaramaConfigByTranportConfig("")
	if err != nil {
		log.Fatalf("failed to get sarama config: %v", err)
	}
	saramaConfig.Producer.Return.Successes = true
	saramaConfig.Producer.MaxMessageBytes = 1024 * 1000 // 1024KB

	sender, err := kafka_sarama.NewSender([]string{bootstrapServer}, saramaConfig, topic)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	defer func() {
		if err := sender.Close(context.Background()); err != nil {
			log.Printf("failed to close sender: %v", err)
		}
	}()

	c, err := cloudevents.NewClient(sender, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	for i := 0; i < count; i++ {
		e := cloudevents.NewEvent()
		e.SetID(uuid.New().String())
		e.SetType("com.cloudevents.sample.sent")
		e.SetSource("https://github.com/cloudevents/sdk-go/samples/kafka/sender")
		e.SetExtension("test", "foo")
		_ = e.SetData(cloudevents.ApplicationJSON, map[string]interface{}{
			"id":      i,
			"message": "Hello, World!",
		})

		if result := c.Send(
			// Set the producer message key
			kafka_sarama.WithMessageKey(context.Background(), sarama.StringEncoder(e.ID())),
			e,
		); cloudevents.IsUndelivered(result) {
			log.Printf("failed to send: %v", result)
		} else {
			log.Printf("sent: %d, accepted: %t", i, cloudevents.IsACK(result))
		}
	}
}
