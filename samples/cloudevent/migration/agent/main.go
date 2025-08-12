package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

// go run ./samples/cloudevent/migration/agent/main.go "hub1" "Registering" "1234567890"
func main() {
	if len(os.Args) < 3 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	hubName := os.Args[1]
	stage := os.Args[2]
	migrationId := os.Args[3]

	ctx, cancel := context.WithCancel(context.Background())
	configMap, err := config.GetConfluentConfigMapByTransportConfig("multicluster-global-hub-agent", "")
	if err != nil {
		log.Fatalf("failed to create protocol: %s", err.Error())
	}
	// exactly once - producer
	configMap.SetKey("enable.idempotence", "true")
	configMap.SetKey("acks", "all")
	configMap.SetKey("retries", "3")
	configMap.SetKey("max.in.flight.requests.per.connection", "5")
	// Sends messages immediately, without waiting to batch them. This reduces latency but can reduce throughput.
	configMap.SetKey("linger.ms", "0")

	sender, err := kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithSenderTopic(fmt.Sprintf("gh-status.%s", hubName)))
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

	bundle := &migration.MigrationStatusBundle{
		MigrationId: migrationId,
		Stage:       stage,
		ErrMessage:  "mock the error message in hub",
	}
	payloadBytes, err := json.Marshal(bundle)
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}

	version := eventversion.NewVersion()

	version.Incr()

	clusterName := constants.CloudEventGlobalHubClusterName
	e := utils.ToCloudEvent(string(enum.ManagedClusterMigrationType), hubName, clusterName, payloadBytes)
	e.SetExtension(eventversion.ExtVersion, version.String())

	if result := c.Send(
		kafka_confluent.WithMessageKey(context.Background(), "test1"),
		e,
	); cloudevents.IsUndelivered(result) {
		log.Printf("failed to send: %v", result)
	} else {
		log.Printf("sent accepted: %t", cloudevents.IsACK(result))
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
