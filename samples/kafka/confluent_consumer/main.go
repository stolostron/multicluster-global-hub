package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stolostron/multicluster-global-hub/samples/config"
)

const (
	messageCount  = 10
	consumerId    = "test-consumer"
	pollTimeoutMs = 100
)

var kafkaMessages = make([]*kafka.Message, 0)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGKILL)

	if len(os.Args) < 2 {
		fmt.Println("Please provide at least one topic command-line argument.")
		os.Exit(1)
	}
	topic := os.Args[1]

	// kafkaConfigMap, err := config.GetConfluentConfigMap()
	kafkaConfigMap, err := config.GetConfluentConfigMapByKafkaUser("KAFKA_USER", false)
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}
	_ = kafkaConfigMap.SetKey("client.id", consumerId)
	_ = kafkaConfigMap.SetKey("group.id", consumerId)
	_ = kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	_ = kafkaConfigMap.SetKey("enable.auto.commit", "true")

	consumer, err := kafka.NewConsumer(kafkaConfigMap)
	if err != nil {
		log.Fatalf("failed to create kafka consumer: %v", err)
	}

	messageChan := make(chan *kafka.Message)
	log.Printf(">> subscribe topic %s", topic)
	if err := consumer.SubscribeTopics([]string{topic}, nil); err != nil {
		log.Fatalf("failed to subscribe topic: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				_ = consumer.Unsubscribe()
				log.Printf("unsubscribed topic: %s", topic)
				return
			default:
				msg, err := consumer.ReadMessage(pollTimeoutMs)
				if err != nil {
					log.Printf("failed to read message: %v", err)
					continue
				}
				if msg == nil {
					// log.Println("msg is nil")
					continue
				}
				messageChan <- msg
			}
		}
	}()

	// committer := NewCommitter(5*time.Second, topic, consumer)
	// committer := NewCommitter(5*time.Second, topic, consumer, getKafkaMessages)
	// committer.start(ctx)

L:
	for {
		select {
		case sig := <-signals:
			log.Printf("got signal: %s\n", sig.String())
			cancel()
			log.Println("context is done")
			break L
		case msg := <-messageChan:
			log.Printf("received message: partition=%d offset=%d val=%s\n", msg.TopicPartition.Partition,
				msg.TopicPartition.Offset, msg.Value)
			if offset := msg.TopicPartition.Offset; offset%2 == 0 {
				kafkaMessages = append(kafkaMessages, msg)
			}
		}
	}

	time.Sleep(1 * time.Second)
	log.Printf("exit main")
	os.Exit(0)
}

func getKafkaMessages() []*kafka.Message {
	return kafkaMessages
}
