package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

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
