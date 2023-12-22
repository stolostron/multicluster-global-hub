package main

import (
	"context"
	"fmt"
	"log"
	"os"

	cloudevents "github.com/cloudevents/sdk-go/v2"
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

	configmap, err := config.GetConfluentConfigMapByKafkaUser(false)
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}

	receiver, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configmap),
		kafka_confluent.WithReceiverTopics([]string{topic}))

	defer receiver.Close(context.Background())

	c, err := cloudevents.NewClient(receiver, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
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
