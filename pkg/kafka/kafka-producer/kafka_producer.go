package kafkaproducer

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"

	kafkaheaders "github.com/stolostron/multicluster-globalhub/pkg/kafka/headers"
	"github.com/stolostron/multicluster-globalhub/pkg/kafka/kafka-producer/builder"
)

const (
	producerFlushPeriod = 5 * int(time.Second)
	intSize             = 4
)

// NewKafkaProducer returns a new instance of KafkaProducer.
//
// Arguments:
// configMap: kafka producer's configuration map.
// messageSizeLimit: the message size limit in bytes (payloads of higher length are broken into fragments).
// deliveryChan: the channel to delivery production events to.
func NewKafkaProducer(configMap *kafka.ConfigMap, messageSizeLimit int,
	deliveryChan chan kafka.Event,
) (*KafkaProducer, error) {
	producer, err := kafka.NewProducer(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create kafka producer - %w", err)
	}

	return &KafkaProducer{
		kafkaProducer:    producer,
		messageSizeLimit: messageSizeLimit,
		deliveryChan:     deliveryChan,
	}, nil
}

// KafkaProducer abstracts Confluent Kafka usage.
type KafkaProducer struct {
	kafkaProducer    *kafka.Producer
	messageSizeLimit int // message size limit in bytes
	deliveryChan     chan kafka.Event
}

// Close closes the KafkaProducer.
func (producer *KafkaProducer) Close() {
	producer.kafkaProducer.Flush(producerFlushPeriod) // give the kafka-producer a chance to finish sending
	producer.kafkaProducer.Close()
}

// Producer returns the wrapped Confluent KafkaProducer.
func (producer *KafkaProducer) Producer() *kafka.Producer {
	return producer.kafkaProducer
}

// ProduceAsync sends a message to the kafka brokers asynchronously.
func (producer *KafkaProducer) ProduceAsync(key string, topic string, partition int32, headers []kafka.Header,
	payload []byte,
) error {
	messageFragments := producer.getMessageFragments(key, &topic, partition, headers, payload)

	for _, message := range messageFragments {
		if err := producer.kafkaProducer.Produce(message, producer.deliveryChan); err != nil {
			return fmt.Errorf("failed to produce message - %w", err)
		}
	}

	return nil
}

func (producer *KafkaProducer) getMessageFragments(key string, topic *string, partition int32, headers []kafka.Header,
	payload []byte,
) []*kafka.Message {
	if len(payload) <= producer.messageSizeLimit {
		return []*kafka.Message{builder.NewMessageBuilder(key, topic, partition, headers, payload).Build()}
	}
	// else, message size is above the limit. need to split the message into fragments.
	chunks := producer.splitPayloadIntoChunks(payload)
	messageFragments := make([]*kafka.Message, len(chunks))
	fragmentationTimestamp := time.Now().Format(time.RFC3339)

	for index, chunk := range chunks {
		messageFragments[index] = builder.NewMessageBuilder(
			fmt.Sprintf("%s_%d", key, index), topic, partition,
			headers, chunk).
			Header(kafka.Header{
				Key: kafkaheaders.Size, Value: toByteArray(len(payload)),
			}).
			Header(kafka.Header{
				Key: kafkaheaders.Offset, Value: toByteArray(index * producer.messageSizeLimit),
			}).
			Header(kafka.Header{
				Key: kafkaheaders.FragmentationTimestamp, Value: []byte(fragmentationTimestamp),
			}).
			Build()
	}

	return messageFragments
}

func (producer *KafkaProducer) splitPayloadIntoChunks(payload []byte) [][]byte {
	var chunk []byte

	chunks := make([][]byte, 0, len(payload)/producer.messageSizeLimit+1)

	for len(payload) >= producer.messageSizeLimit {
		chunk, payload = payload[:producer.messageSizeLimit], payload[producer.messageSizeLimit:]
		chunks = append(chunks, chunk)
	}

	if len(payload) > 0 {
		chunks = append(chunks, payload)
	}

	return chunks
}

func toByteArray(i int) []byte {
	arr := make([]byte, intSize)
	binary.BigEndian.PutUint32(arr[0:intSize], uint32(i))

	return arr
}
