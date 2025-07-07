package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/samples/config"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
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
		processBundle[clusterv1.ManagedCluster](event)
	case string(enum.LocalPolicySpecType):
		processBundle[policiesv1.Policy](event)
	default:
		// // unknown types
		// payload, _ := json.MarshalIndent(event, "", "  ")
		// fmt.Println(string(payload))
	}
}

func processBundle[T any](event cloudevents.Event) {
	var bundle generic.GenericBundle[T]
	if err := event.DataAs(&bundle); err != nil {
		log.Printf("----- error decoding event [%s] ----\n", event.Type())
		utils.PrettyPrint(event)
		log.Println("----- error end ----")
		return
	}

	shortType := strings.TrimPrefix(event.Type(), enum.EventTypePrefix)
	version := event.Extensions()["extversion"]

	fmt.Printf("%s - %s: create %d, update %d, delete %d, resync %d, metadata %d\n",
		shortType, version,
		len(bundle.Create), len(bundle.Update), len(bundle.Delete),
		len(bundle.Resync), len(bundle.ResyncMetadata),
	)
}
