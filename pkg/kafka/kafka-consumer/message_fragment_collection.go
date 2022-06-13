package kafkaconsumer

import (
	"fmt"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

var errFragmentsAreIncomplete = fmt.Errorf("some fragments are missing")

func newMessageFragmentsCollection(size uint32, timestamp time.Time) *messageFragmentsCollection {
	return &messageFragmentsCollection{
		totalMessageSize:         size,
		accumulatedFragmentsSize: 0,
		fragmentationTimestamp:   timestamp,
		messageFragments:         make(map[uint32]*messageFragment),
		latestKafkaMessage:       nil,
		highestOffset:            0,
		lowestOffset:             -1,
		lock:                     sync.Mutex{},
	}
}

// kafkaMessageFragmentsCollection holds a collection of messageFragment and maintains it until completion.
type messageFragmentsCollection struct {
	totalMessageSize         uint32
	accumulatedFragmentsSize uint32
	fragmentationTimestamp   time.Time
	messageFragments         map[uint32]*messageFragment

	latestKafkaMessage *kafka.Message
	highestOffset      kafka.Offset
	lowestOffset       kafka.Offset
	lock               sync.Mutex
}

func (collection *messageFragmentsCollection) add(fragmentInfo *messageFragmentInfo) {
	collection.lock.Lock()
	defer collection.lock.Unlock()

	if fragmentInfo.fragment.offset+uint32(len(fragmentInfo.fragment.bytes)) > collection.totalMessageSize {
		return // fragment reaches out of message bounds
	}

	// don't add fragment to collection, if already exists. this may happen if kafka uses at least once guarantees
	if _, found := collection.messageFragments[fragmentInfo.fragment.offset]; found {
		return
	}

	collection.messageFragments[fragmentInfo.fragment.offset] = fragmentInfo.fragment

	// update accumulated size.
	collection.accumulatedFragmentsSize += uint32(len(fragmentInfo.fragment.bytes))

	// update offsets.
	if fragmentInfo.kafkaMessage.TopicPartition.Offset >= collection.highestOffset {
		collection.latestKafkaMessage = fragmentInfo.kafkaMessage
		collection.highestOffset = fragmentInfo.kafkaMessage.TopicPartition.Offset
	}

	if collection.lowestOffset == -1 || fragmentInfo.kafkaMessage.TopicPartition.Offset <= collection.lowestOffset {
		collection.lowestOffset = fragmentInfo.kafkaMessage.TopicPartition.Offset
	}
}

// assemble assembles the collection into one bundle.
// This function only runs when totalMessageSize == accumulatedFragmentsSize.
func (collection *messageFragmentsCollection) assemble() ([]byte, error) {
	collection.lock.Lock()
	defer collection.lock.Unlock()

	if collection.totalMessageSize != collection.accumulatedFragmentsSize {
		return nil, errFragmentsAreIncomplete
	}

	buffer := make([]byte, collection.totalMessageSize)

	for offset, fragment := range collection.messageFragments {
		copy(buffer[offset:], fragment.bytes)
		fragment.bytes = nil // faster GC
	}

	return buffer, nil
}
