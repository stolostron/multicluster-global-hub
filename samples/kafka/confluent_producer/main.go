package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

var (
	topic        = "spec"
	messageCount = 10
	producerId   = "test-producer"
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGKILL)

	deliveryChan := make(chan kafka.Event)

	kafkaConfigMap, err := config.GetConfluentConfigMap()
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}
	kafkaConfigMap.SetKey("client.id", producerId)
	kafkaConfigMap.SetKey("acks", "1")
	kafkaConfigMap.SetKey("retries", "0")

	producer, err := kafka.NewProducer(kafkaConfigMap)
	if err != nil {
		log.Fatalf("failed to create kafka producer: %v", err)
	}

	for i := 0; i < messageCount; i++ {
		value := fmt.Sprintf("message-%d", i)
		err := producer.Produce(&kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &topic, Partition: 0},
			Value:          []byte(value),
			Key:            []byte("key"),
			Headers:        []kafka.Header{{Key: "myTestHeader", Value: []byte("header values are binary")}},
		}, deliveryChan)
		if err != nil {
			log.Fatalf("failed to produce message: %v", err)
		}
	}

	// waiting for the end of all messages sent or an OS signal
	for i := 0; i < messageCount*2; i++ {
		select {
		case e := <-deliveryChan:
			kafkaMessage, ok := e.(*kafka.Message)
			if !ok {
				log.Printf("Failed to cast kafka message: %v\n", e)
				continue
			}
			// the offset
			log.Printf("Finished to send: partition=%d offset=%d val=%s\n", kafkaMessage.TopicPartition.Partition, kafkaMessage.TopicPartition.Offset, kafkaMessage.Value)
		case sig := <-signals:
			log.Printf("Got signal: %v\n", sig)
			return
		}
	}
}
