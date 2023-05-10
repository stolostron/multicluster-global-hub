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
	topic         = "spec"
	messageCount  = 10
	consumerId    = "test-consumer"
	pollTimeoutMs = 100
)

var kafkaMessages = make([]*kafka.Message, 0)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGKILL)

	kafkaConfigMap, err := config.GetConfluentConfigMap()
	if err != nil {
		log.Fatalf("failed to get kafka config map: %v", err)
	}
	_ = kafkaConfigMap.SetKey("client.id", consumerId)
	_ = kafkaConfigMap.SetKey("group.id", consumerId)
	// kafkaConfigMap.SetKey("auto.offset.reset", "earliest")
	// kafkaConfigMap.SetKey("enable.auto.commit", "false")

	consumer, err := kafka.NewConsumer(kafkaConfigMap)
	if err != nil {
		log.Fatalf("failed to create kafka consumer: %v", err)
	}
	messageChan := make(chan *kafka.Message)
	if err := consumer.SubscribeTopics([]string{topic}, nil); err != nil {
		log.Fatalf("failed to subscribe topic: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case sig := <-signals:
				log.Printf("received signal: %v", sig)
				_ = consumer.Unsubscribe()
				log.Printf("unsubscribed topic: %s", topic)
				cancel()
				return
			default:
				msg, err := consumer.ReadMessage(pollTimeoutMs)
				if err != nil && err.(kafka.Error).Code() != kafka.ErrTimedOut {
					log.Fatalf("failed to read message: %v", err)
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
	committer := NewCommitter(5*time.Second, topic, consumer, getKafkaMessages)
	committer.start(ctx)
	for {
		select {
		case <-ctx.Done():
			log.Println("context is done")
			return
		case msg := <-messageChan:
			log.Printf("Received message: partition=%d offset=%d val=%s\n", msg.TopicPartition.Partition,
				msg.TopicPartition.Offset, msg.Value)
			if offset := msg.TopicPartition.Offset; offset%2 == 0 {
				kafkaMessages = append(kafkaMessages, msg)
			}
		}
	}
}

func getKafkaMessages() []*kafka.Message {
	return kafkaMessages
}

type committer struct {
	topic           string
	client          *kafka.Consumer
	latestCommitted map[int32]kafka.Offset // map of partition -> offset
	interval        time.Duration
	messageFunc     func() []*kafka.Message
}

// NewCommitter returns a new instance of committer.
func NewCommitter(committerInterval time.Duration, topic string, client *kafka.Consumer,
	messageFunc func() []*kafka.Message,
) *committer {
	return &committer{
		topic:           topic,
		client:          client,
		latestCommitted: make(map[int32]kafka.Offset),
		interval:        committerInterval,
		messageFunc:     messageFunc,
	}
}

func (c *committer) start(ctx context.Context) {
	go c.periodicCommit(ctx)
}

func (c *committer) periodicCommit(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C: // wait for next time interval
			messages := c.messageFunc()
			if err := c.commitOffsets(messages); err != nil {
				log.Printf("failed to commit offsets: %v", err)
			}
		}
	}
}

// commitOffsets commits the given offsets per partition mapped.
func (c *committer) commitOffsets(messages []*kafka.Message) error {
	for _, msg := range messages {
		partition := msg.TopicPartition.Partition
		offset := msg.TopicPartition.Offset
		// skip request if already committed this offset
		if committedOffset, found := c.latestCommitted[partition]; found {
			if committedOffset >= offset {
				continue
			}
		}

		topicPartition := kafka.TopicPartition{
			Topic:     &c.topic,
			Partition: partition,
			Offset:    offset,
		}

		if _, err := c.client.CommitOffsets([]kafka.TopicPartition{
			topicPartition,
		}); err != nil {
			return fmt.Errorf("failed to commit offset, stopping bulk commit - %w", err)
		}

		log.Printf("committed topic %s, partition %d, offset %d", c.topic, partition, offset)
		// update commitsMap
		c.latestCommitted[partition] = offset
	}

	return nil
}
