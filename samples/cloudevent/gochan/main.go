package main

import (
	"context"
	"fmt"
	"log"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/producer"
)

var transportConfig = &transport.TransportInternalConfig{
	TransportType: string(transport.Chan),
	KafkaCredential: &transport.KafkaConfig{
		SpecTopic: "spec",
	},
}

func main() {
	consumer, err := consumer.NewGenericConsumer(transportConfig, []string{"spec"})
	if err != nil {
		log.Fatalf("failed to create consumer: %v", err)
	}

	go func() {
		err = consumer.Start(context.Background())
		if err != nil {
			log.Fatalf("failed to start consumer: %v", err)
		}
	}()

	go func() {
		p, err := producer.NewGenericProducer(transportConfig, "spec", nil)
		if err != nil {
			log.Fatalf("failed to create producer: %v", err)
		}

		event := cloudevents.NewEvent()
		event.SetType("test")
		event.SetSource("test")
		event.SetData("application/json", "test-data")

		err = p.SendEvent(context.Background(), event)
		if err != nil {
			log.Fatalf("failed to send event: %v", err)
		}
		fmt.Println("Sent event")
	}()

	for event := range consumer.EventChan() {
		fmt.Println("Received event:")
		fmt.Println(event)
	}
}
