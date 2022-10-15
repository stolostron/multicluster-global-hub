package producer

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/helpers"
)

const (
	partition           = 0
	kiloBytesToBytes    = 1000
	producerFlushPeriod = 5 * int(time.Second)
	intSize             = 4
	MaxMessageSizeLimit = 987
)

type KafkaProducerConfig struct {
	ProducerID     string
	ProducerTopic  string
	MsgSizeLimitKB int
}

// Producer abstracts hub-of-hubs/pkg/kafka kafka-producer's generic usage.
type KafkaProducer struct {
	log                  logr.Logger
	producer             *kafka.Producer
	messageSizeLimit     int // message size limit in bytes
	eventSubscriptionMap map[string]map[EventType]EventCallback
	topic                string
	compressor           compressor.Compressor
	deliveryChan         chan kafka.Event
	stopChan             chan struct{}
	startOnce            sync.Once
	stopOnce             sync.Once
}

// NewProducer returns a new instance of Producer object.
func NewKafkaProducer(compressor compressor.Compressor, bootstrapServer, sslCA string,
	producerConfig *KafkaProducerConfig, log logr.Logger,
) (*KafkaProducer, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       bootstrapServer,
		"client.id":               producerConfig.ProducerID,
		"acks":                    "1",
		"retries":                 "0",
		"socket.keepalive.enable": "true",
		"log.connection.close":    "false", // silence spontaneous disconnection logs, kafka recovers by itself.
	}

	err := helpers.LoadSslToConfigMap(sslCA, kafkaConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to configure kafka-producer - %w", err)
	}

	producer, err := kafka.NewProducer(kafkaConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &KafkaProducer{
		log:                  log,
		producer:             producer,
		messageSizeLimit:     producerConfig.MsgSizeLimitKB,
		topic:                producerConfig.ProducerTopic,
		eventSubscriptionMap: make(map[string]map[EventType]EventCallback),
		compressor:           compressor,
		deliveryChan:         make(chan kafka.Event),
		stopChan:             make(chan struct{}),
	}, nil
}

// Start starts the kafka.
func (p *KafkaProducer) Start() {
	p.startOnce.Do(func() {
		go p.deliveryReportHandler()
	})
}

// Stop stops the producer.
func (p *KafkaProducer) Stop() {
	p.stopOnce.Do(func() {
		p.stopChan <- struct{}{}
		close(p.deliveryChan)
		close(p.stopChan)
		p.producer.Close()
	})
}

func (p *KafkaProducer) deliveryReportHandler() {
	for {
		select {
		case <-p.stopChan:
			return

		case event := <-p.deliveryChan:
			kafkaMessage, ok := event.(*kafka.Message)
			if !ok {
				p.log.Info("received unsupported kafka-event type", "event type", event)
				continue
			}
			p.handleDeliveryReport(kafkaMessage)
		}
	}
}

// handleDeliveryReport handles results of sent messages.
func (p *KafkaProducer) handleDeliveryReport(kafkaMessage *kafka.Message) {
	if kafkaMessage.TopicPartition.Error != nil {
		p.log.Error(kafkaMessage.TopicPartition.Error, "failed to deliver message",
			"MessageId", string(kafkaMessage.Key), "TopicPartition", kafkaMessage.TopicPartition)
		InvokeCallback(p.eventSubscriptionMap, string(kafkaMessage.Key), DeliveryFailure)
		return
	}

	InvokeCallback(p.eventSubscriptionMap, string(kafkaMessage.Key), DeliverySuccess)
}

// Subscribe adds a callback to be delegated when a given event occurs for a message with the given ID.
func (p *KafkaProducer) Subscribe(messageID string, callbacks map[EventType]EventCallback) {
	p.eventSubscriptionMap[messageID] = callbacks
}

// SupportsDeltaBundles returns true. kafka does support delta bundles.
func (p *KafkaProducer) SupportsDeltaBundles() bool {
	return true
}

// SendAsync sends a message to the sync service asynchronously.
func (p *KafkaProducer) SendAsync(msg *transport.Message) {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		p.log.Error(err, "failed to send message", "MessageId", msg.ID, "MessageType", msg.MsgType,
			"Version", msg.Version)

		return
	}

	compressedBytes, err := p.compressor.Compress(msgBytes)
	if err != nil {
		p.log.Error(err, "failed to compress bundle", "CompressorType", p.compressor.GetType(),
			"MessageId", msg.ID, "MessageType", msg.MsgType, "Version", msg.Version)

		return
	}

	messageHeaders := []kafka.Header{
		{Key: transport.CompressionType, Value: []byte(p.compressor.GetType())},
	}

	// It was from manager producer. Need decide if we need it for not.
	// msgKey := msg.ID
	// if destinationHubName != transport.Broadcast { // set destination if specified
	// 	msgKey = fmt.Sprintf("%s.%s", destinationHubName, msg.ID)

	// 	messageHeaders = append(messageHeaders, kafka.Header{
	// 		Key:   headers.DestinationHub,
	// 		Value: []byte(destinationHubName),
	// 	})
	// }

	if err = p.produceAsync(msg.Key, p.topic, partition, messageHeaders, compressedBytes); err != nil {
		p.log.Error(err, "failed to send message", "MessageKey", msg.Key, "MessageId", msg.ID,
			"MessageType", msg.MsgType, "Version", msg.Version)
		InvokeCallback(p.eventSubscriptionMap, string(msg.ID), DeliveryFailure)

		return
	}
	InvokeCallback(p.eventSubscriptionMap, string(msg.ID), DeliveryAttempt)
	p.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType, "Version", msg.Version)
}

// Register function registers a bundle ID to a CustomBundleRegistration.
func (c *KafkaProducer) CustomBundleRegister(msgID string, customBundleRegistration *registration.CustomBundleRegistration) {

}

// Register function registers a msgID to the bundle updates channel.
func (c *KafkaProducer) BundleRegister(registration *registration.BundleRegistration) {

}

// Close closes the KafkaProducer.
func (p *KafkaProducer) Close() {
	p.producer.Flush(producerFlushPeriod) // give the kafka-producer a chance to finish sending
	p.producer.Close()
}

// Producer returns the wrapped Confluent KafkaProducer.
func (p *KafkaProducer) Producer() *kafka.Producer {
	return p.producer
}

// ProduceAsync sends a message to the kafka brokers asynchronously.
func (p *KafkaProducer) produceAsync(key string, topic string, partition int32, headers []kafka.Header,
	payload []byte,
) error {
	messageFragments := p.getMessageFragments(key, &topic, partition, headers, payload)

	for _, message := range messageFragments {
		if err := p.producer.Produce(message, p.deliveryChan); err != nil {
			return fmt.Errorf("failed to produce message - %w", err)
		}
	}

	return nil
}

func (p *KafkaProducer) getMessageFragments(key string, topic *string, partition int32, headers []kafka.Header,
	payload []byte,
) []*kafka.Message {
	if len(payload) <= p.messageSizeLimit {
		return []*kafka.Message{NewMessageBuilder(key, topic, partition, headers, payload).Build()}
	}
	// else, message size is above the limit. need to split the message into fragments.
	chunks := p.splitPayloadIntoChunks(payload)
	messageFragments := make([]*kafka.Message, len(chunks))
	fragmentationTimestamp := time.Now().Format(time.RFC3339)

	for index, chunk := range chunks {
		messageFragments[index] = NewMessageBuilder(
			fmt.Sprintf("%s_%d", key, index), topic, partition,
			headers, chunk).
			Header(kafka.Header{
				Key: transport.Size, Value: helpers.ToByteArray(len(payload)),
			}).
			Header(kafka.Header{
				Key: transport.Offset, Value: helpers.ToByteArray(index * p.messageSizeLimit),
			}).
			Header(kafka.Header{
				Key: transport.FragmentationTimestamp, Value: []byte(fragmentationTimestamp),
			}).
			Build()
	}

	return messageFragments
}

func (p *KafkaProducer) splitPayloadIntoChunks(payload []byte) [][]byte {
	var chunk []byte

	chunks := make([][]byte, 0, len(payload)/p.messageSizeLimit+1)

	for len(payload) >= p.messageSizeLimit {
		chunk, payload = payload[:p.messageSizeLimit], payload[p.messageSizeLimit:]
		chunks = append(chunks, chunk)
	}

	if len(payload) > 0 {
		chunks = append(chunks, payload)
	}

	return chunks
}
