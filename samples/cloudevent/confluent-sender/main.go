package main

import (
	"context"
	"fmt"
	"log"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
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

	configmap, err := config.GetConfluentConfigMapByKafkaUser(true)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	sender, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configmap),
		kafka_confluent.WithSenderTopic(topic))

	defer sender.Close(context.Background())

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
			"message": fmt.Sprintf("Hello, %s!", topic),
		})

		if result := c.Send(
			kafka_confluent.WithMessageKey(context.Background(), e.ID()),
			e,
		); cloudevents.IsUndelivered(result) {
			log.Printf("failed to send: %v", result)
		} else {
			log.Printf("sent: %d, accepted: %t", i, cloudevents.IsACK(result))
		}
	}
}
