package kafkaconsumer

import (
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

// messageFragmentInfo wraps a fragment with info to pass in channels.
type messageFragmentInfo struct {
	key                    string
	totalSize              uint32
	fragmentationTimestamp time.Time
	fragment               *messageFragment
	kafkaMessage           *kafka.Message
}
