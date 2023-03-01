package consumer

import (
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
)

// messageFragment represents one fragment of a kafka message.
type messageFragment struct {
	offset uint32
	bytes  []byte
}

// messageFragmentInfo wraps a fragment with info to pass in channels.
type messageFragmentInfo struct {
	key                    string
	totalSize              uint32
	fragmentationTimestamp time.Time
	fragment               *messageFragment
	kafkaMessage           *kafka.Message
}
