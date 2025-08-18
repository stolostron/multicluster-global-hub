package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

var (
	messageCount = 5
	producerId   = "test-producer"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]

	// kafkaConfigMap, err := config.GetConfluentConfigMap()
	kafkaConfigMap, err := config.GetConfluentConfigMap(true)
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}
	_ = kafkaConfigMap.SetKey("client.id", producerId)

	producer, err := kafka.NewProducer(kafkaConfigMap)
	if err != nil {
		log.Fatalf("failed to create kafka producer: %v", err)
	}

	// Listen to all the events on the default events channel
	go func() {
		count := 0
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				// The message delivery report, indicating success or
				// permanent failure after retries have been exhausted.
				// Application level retries won't help since the client
				// is already configured to do that.
				m := ev
				if m.TopicPartition.Error != nil {
					log.Printf("delivery failed: %v\n", m.TopicPartition.Error)
				} else {
					log.Printf("delivered message to topic %s [%d] at offset %v\n",
						*m.TopicPartition.Topic, m.TopicPartition.Partition, m.TopicPartition.Offset)
				}
				count++
				if count == messageCount {
					signals <- syscall.SIGKILL
					return
				}
			case kafka.Error:
				// Generic client instance-level errors, such as
				// broker connection failures, authentication issues, etc.
				//
				// These errors should generally be considered informational
				// as the underlying client will automatically try to
				// recover from any errors encountered, the application
				// does not need to take action on them.
				log.Printf("kafka error: %v\n", ev)
			default:
				log.Printf("ignored event: %s\n", ev)
			}
		}
	}()

	for i := 0; i < messageCount; i++ {
		value := fmt.Sprintf("message-%s", topic)
		err := producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0},
			Value:          []byte(value),
			Key:            []byte("key"),
			Headers:        []kafka.Header{{Key: "myTestHeader", Value: []byte("header values are binary")}},
		}, nil)
		if err != nil {
			if err.(kafka.Error).Code() == kafka.ErrQueueFull {
				// Producer queue is full, wait 1s for messages
				// to be delivered then try again.
				time.Sleep(time.Second)
				continue
			}
			log.Fatalf("failed to produce message: %v\n", err)
		}
	}

	sig := <-signals
	log.Printf("got signal: %v\n", sig)
	producer.Close()
	log.Println("close producer")
	time.Sleep(1 * time.Second)
}
