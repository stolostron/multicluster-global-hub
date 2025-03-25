package main

import (
	"context"
	"fmt"
	"log"
	"os"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
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

	ctx, cancel := context.WithCancel(context.Background())
	configmap, err := config.GetConfluentConfigMapByTransportConfig("multicluster-global-hub-agent", "")
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}
	sender, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configmap),
		kafka_confluent.WithSenderTopic(topic))
	if err != nil {
		log.Fatalf("failed to create protocol, %v", err)
	}
	defer sender.Close(ctx)
	eventChan, _ := sender.Events()
	handleProducerEvents(eventChan)

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
	cancel()
}

func handleProducerEvents(eventChan chan kafka.Event) {
	go func() {
		for e := range eventChan {
			switch ev := e.(type) {
			case *kafka.Message:
				// The message delivery report, indicating success or
				// permanent failure after retries have been exhausted.
				// Application level retries won't help since the client
				// is already configured to do that.
				m := ev
				if m.TopicPartition.Error != nil {
					log.Printf("Delivery failed: %v\n", m.TopicPartition.Error)
				} else {
					log.Printf("Delivered message %v\n", string(m.Value))
					// log.Printf("Delivered message to topic %s [%d] at offset %v\n",
					// 	*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
				}
			case kafka.Error:
				// Generic client instance-level errors, such as
				// broker connection failures, authentication issues, etc.
				//
				// These errors should generally be considered informational
				// as the underlying client will automatically try to
				// recover from any errors encountered, the application
				// does not need to take action on them.
				log.Printf("Error: %v\n", ev)
			default:
				log.Printf("Ignored event: %v\n", ev)
			}
		}
	}()
}
