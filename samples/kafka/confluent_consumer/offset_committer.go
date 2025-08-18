package main

import (
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

