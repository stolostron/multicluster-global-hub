package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
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
	ctx := context.Background()

	configMap, err := config.GetConfluentConfigMapByTransportConfig(os.Getenv("KAFKA_NAMESPACE"), "test-consumer-Id")
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}
	receiver, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithReceiverTopics([]string{topic}))
	if err != nil {
		log.Fatalf("failed to subscribe topic: %v", err)
	}

	defer receiver.Close(ctx)

	c, err := cloudevents.NewClient(receiver, cloudevents.WithTimeNow(), cloudevents.WithUUIDs(),
		client.WithPollGoroutines(1), client.WithBlockingCallback())
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	// topic1 := "status.hub1"
	// topic2 := "status.hub2"
	// offsetToStart := []kafka.TopicPartition{
	// 	{Topic: &topic1, Partition: 0, Offset: 5},
	// 	{Topic: &topic2, Partition: 0, Offset: 5},
	// }
	// ctx := kafka_confluent.CommitOffsetCtx(context.Background(), offsetToStart)

	log.Printf("will listen consuming topic: %s\n", topic)

	err = c.StartReceiver(cectx.WithLogger(ctx, logger.ZapLogger("cloudevents")), receive)
	if err != nil {
		log.Fatalf("failed to start receiver: %s", err)
	} else {
		log.Printf("receiver stopped\n")
	}
}

func receive(ctx context.Context, event cloudevents.Event) {
	payload, _ := json.MarshalIndent(event, "", "  ")
	fmt.Println(string(payload))
}
