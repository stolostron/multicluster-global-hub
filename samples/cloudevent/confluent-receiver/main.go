package main

import (
	"context"
	"fmt"
	"log"
	"os"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]
	ctx := context.Background()

	configMap, err := config.GetConfluentConfigMapByTransportConfig("multicluster-global-hub-agent", "transport-config",
		"test-consumer-id-2")
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}
	// kafkaNamespace := "kafka"
	// kafkaCluster := "kafka"
	// kafkaUser := "my-user"
	// configMap, err := config.GetConfluentConfigMapByCurrentUser(kafkaNamespace, kafkaCluster, kafkaUser)
	// if err != nil {
	// 	log.Fatalf("failed to create configmap: %s", err.Error())
	// }
	// _ = configMap.SetKey("group.id", "consumergroup-test-2")
	_ = configMap.SetKey("auto.offset.reset", "earliest")
	_ = configMap.SetKey("enable.auto.commit", "true")

	receiver, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithReceiverTopics([]string{topic}))
	if err != nil {
		log.Fatalf("failed to subscribe topic: %v", err)
	}

	defer func() {
		if err := receiver.Close(ctx); err != nil {
			log.Printf("failed to close receiver: %v", err)
		}
	}()

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

	err = c.StartReceiver(cectx.WithLogger(ctx, logger.ZapLogger("cloudevents")), handleEvent)
	if err != nil {
		log.Fatalf("failed to start receiver: %s", err)
	} else {
		log.Printf("receiver stopped\n")
	}
}

func handleEvent(ctx context.Context, event cloudevents.Event) {
	switch event.Type() {
	case string(enum.ManagedClusterType):
		fmt.Println(event)
	case string(enum.LocalPolicySpecType):
		fmt.Println(event)
	case string(enum.ManagedClusterMigrationType):
		fmt.Println(event)
	case string(enum.HubClusterHeartbeatType):
		// do nothing
	default:
		fmt.Println(event)
	}
}
