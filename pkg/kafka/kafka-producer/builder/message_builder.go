package builder

import (
	"github.com/confluentinc/confluent-kafka-go/kafka"
)

// NewMessageBuilder creates a new instance of MessageBuilder.
func NewMessageBuilder(key string, topic *string, partitionID int32, headers []kafka.Header,
	payload []byte) *MessageBuilder {
	return &MessageBuilder{
		message: &kafka.Message{
			Key: []byte(key),
			TopicPartition: kafka.TopicPartition{
				Topic:     topic,
				Partition: partitionID,
			},
			Headers: headers,
			Value:   payload,
		},
	}
}

// MessageBuilder uses the builder patten to construct a kafka message.
type MessageBuilder struct {
	message *kafka.Message
}

// Header adds a header to the message headers.
func (builder *MessageBuilder) Header(header kafka.Header) *MessageBuilder {
	builder.message.Headers = append(builder.message.Headers, header)

	return builder
}

// Build returns the internal kafka message.
func (builder *MessageBuilder) Build() *kafka.Message {
	return builder.message
}
