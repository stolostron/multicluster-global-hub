package producer

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs-kafka-transport/headers"
	kafkaproducer "github.com/stolostron/hub-of-hubs-kafka-transport/kafka-client/kafka-producer"
	"github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	"github.com/stolostron/hub-of-hubs/pkg/compressor"
)

const (
	partition        = 0
	kiloBytesToBytes = 1000
)

// Producer abstracts hub-of-hubs-kafka-transport kafka-producer's generic usage.
type KafkaProducer struct {
	log                  logr.Logger
	kafkaProducer        *kafkaproducer.KafkaProducer
	eventSubscriptionMap map[string]map[EventType]EventCallback
	topic                string
	compressor           compressor.Compressor
	deliveryChan         chan kafka.Event
	stopChan             chan struct{}
	startOnce            sync.Once
	stopOnce             sync.Once
}

// NewProducer returns a new instance of Producer object.
func NewKafkaProducer(compressor compressor.Compressor, log logr.Logger, environmentManager *helper.ConfigManager) (*KafkaProducer, error) {
	configMap, err := environmentManager.GetProducerKafkaConfigMap()
	if err != nil {
		return nil, fmt.Errorf("failed to get kafka configMap")
	}

	deliveryChan := make(chan kafka.Event)
	kafkaProducer, err := kafkaproducer.NewKafkaProducer(configMap, environmentManager.Kafka.ProducerMessageLimit*kiloBytesToBytes, deliveryChan)
	if err != nil {
		close(deliveryChan)
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	return &KafkaProducer{
		log:                  log,
		kafkaProducer:        kafkaProducer,
		topic:                environmentManager.Kafka.ProducerTopic,
		eventSubscriptionMap: make(map[string]map[EventType]EventCallback),
		compressor:           compressor,
		deliveryChan:         deliveryChan,
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
		p.kafkaProducer.Close()
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
func (p *KafkaProducer) SendAsync(msg *Message) {
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
		{Key: headers.CompressionType, Value: []byte(p.compressor.GetType())},
	}

	if err = p.kafkaProducer.ProduceAsync(msg.Key, p.topic, partition, messageHeaders, compressedBytes); err != nil {
		p.log.Error(err, "failed to send message", "MessageKey", msg.Key, "MessageId", msg.ID,
			"MessageType", msg.MsgType, "Version", msg.Version)
		InvokeCallback(p.eventSubscriptionMap, string(msg.ID), DeliveryFailure)

		return
	}
	InvokeCallback(p.eventSubscriptionMap, string(msg.ID), DeliveryAttempt)
	p.log.Info("Message sent successfully", "MessageId", msg.ID, "MessageType", msg.MsgType, "Version", msg.Version)
}
