package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs-kafka-transport/headers"
	kafkaconsumer "github.com/stolostron/hub-of-hubs-kafka-transport/kafka-client/kafka-consumer"
	helper "github.com/stolostron/hub-of-hubs/agent/pkg/helper"
	bundle "github.com/stolostron/hub-of-hubs/agent/pkg/spec/bundle"
	"github.com/stolostron/hub-of-hubs/agent/pkg/transport"
	"github.com/stolostron/hub-of-hubs/pkg/compressor"
)

// Consumer abstracts hub-of-hubs-kafka-transport kafka-consumer's generic usage.
type KafkaComsumer struct {
	log            logr.Logger
	leafHubName    string
	kafkaConsumer  *kafkaconsumer.KafkaConsumer
	compressorsMap map[compressor.CompressionType]compressor.Compressor
	topic          string

	messageChan                     chan *kafka.Message
	genericBundlesChan              chan *bundle.GenericBundle // messageChan get the message from kafka and put it to the genericBundleChan
	customBundleIDToRegistrationMap map[string]*bundle.CustomBundleRegistration

	partitionToOffsetToCommitMap map[int32]kafka.Offset // size limited at all times (low)

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once
	lock       sync.Mutex
}

// NewConsumer creates a new instance of Consumer.
func NewKafkaConsumer(log logr.Logger, environmentManager *helper.ConfigManager, genericBundlesChan chan *bundle.GenericBundle) (*KafkaComsumer, error) {
	leafHubName := environmentManager.LeafHubName
	topic := environmentManager.Kafka.ComsumerTopic

	kafkaConfigMap, err := environmentManager.GetKafkaConfigMap()
	if err != nil {
		return nil, fmt.Errorf("failed to get kafka configMap: %w", err)
	}

	messageChan := make(chan *kafka.Message)
	kafkaConsumer, err := kafkaconsumer.NewKafkaConsumer(kafkaConfigMap, messageChan, log)
	if err != nil {
		close(messageChan)
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	if err := kafkaConsumer.Subscribe(topic); err != nil {
		close(messageChan)
		kafkaConsumer.Close()
		return nil, fmt.Errorf("failed to subscribe to requested topic - %v: %w", topic, err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

	return &KafkaComsumer{
		log:                             log,
		leafHubName:                     leafHubName,
		kafkaConsumer:                   kafkaConsumer,
		compressorsMap:                  make(map[compressor.CompressionType]compressor.Compressor),
		topic:                           topic,
		messageChan:                     messageChan,
		genericBundlesChan:              genericBundlesChan,
		customBundleIDToRegistrationMap: make(map[string]*bundle.CustomBundleRegistration),
		partitionToOffsetToCommitMap:    make(map[int32]kafka.Offset),
		ctx:                             ctx,
		cancelFunc:                      cancelFunc,
		lock:                            sync.Mutex{},
	}, nil
}

// Start function starts the consumer.
func (c *KafkaComsumer) Start() {
	c.startOnce.Do(func() {
		go c.handleKafkaMessages(c.ctx)
	})
}

// Stop stops the consumer.
func (c *KafkaComsumer) Stop() {
	c.stopOnce.Do(func() {
		c.cancelFunc()
		close(c.messageChan)
		c.kafkaConsumer.Close()
	})
}

// Register function registers a bundle ID to a CustomBundleRegistration.
func (c *KafkaComsumer) Register(msgID string, customBundleRegistration *bundle.CustomBundleRegistration) {
	c.customBundleIDToRegistrationMap[msgID] = customBundleRegistration
}

func (c *KafkaComsumer) handleKafkaMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-c.messageChan:
			c.log.Info("received message and forward to bundle chan...")
			c.processMessage(msg)
		}
	}
}

func (c *KafkaComsumer) processMessage(message *kafka.Message) {
	if msgDestinationLeafHubBytes, found := c.lookupHeaderValue(message, headers.DestinationHub); found {
		if string(msgDestinationLeafHubBytes) != c.leafHubName {
			return // if destination is explicitly specified and does not match, drop bundle
		}
	} // if header is not found then assume broadcast

	compressionTypeBytes, found := c.lookupHeaderValue(message, headers.CompressionType)
	if !found {
		c.logError(errors.New("compression type is missing from message description"), "failed to read bundle", message)
		return
	}

	decompressedPayload, err := c.decompressPayload(message.Value, compressor.CompressionType(compressionTypeBytes))
	if err != nil {
		c.logError(err, "failed to decompress bundle bytes", message)
		return
	}

	transportMessage := &transport.Message{}
	if err := json.Unmarshal(decompressedPayload, transportMessage); err != nil {
		c.logError(err, "failed to parse transport message", message)
		return
	}

	customBundleRegistration, found := c.customBundleIDToRegistrationMap[transportMessage.ID]
	if !found { // received generic bundle
		if err := c.syncGenericBundle(transportMessage.Payload); err != nil {
			c.log.Error(err, "failed to parse bundle", "MessageID", transportMessage.ID,
				"MessageType", transportMessage.MsgType, "Version", transportMessage.Version)
		}
		return
	}
	// received a custom bundle
	if err := c.SyncCustomBundle(customBundleRegistration, transportMessage.Payload); err != nil {
		c.log.Error(err, "failed to parse bundle", "MessageID", transportMessage.ID,
			"MessageType", transportMessage.MsgType, "Version", transportMessage.Version)
	}
}

func (c *KafkaComsumer) syncGenericBundle(payload []byte) error {
	receivedBundle := bundle.NewGenericBundle()
	if err := json.Unmarshal(payload, receivedBundle); err != nil {
		return fmt.Errorf("failed to parse bundle - %w", err)
	}
	c.genericBundlesChan <- receivedBundle
	return nil
}

// SyncCustomBundle writes a custom bundle to its respective syncer channel.
func (c *KafkaComsumer) SyncCustomBundle(customBundleRegistration *bundle.CustomBundleRegistration, payload []byte) error {
	receivedBundle := customBundleRegistration.InitBundlesResourceFunc()
	if err := json.Unmarshal(payload, &receivedBundle); err != nil {
		return fmt.Errorf("failed to parse custom bundle - %w", err)
	}
	customBundleRegistration.BundleUpdatesChan <- receivedBundle
	return nil
}

func (c *KafkaComsumer) logError(err error, errMessage string, msg *kafka.Message) {
	c.log.Error(err, errMessage, "MessageKey", string(msg.Key), "TopicPartition", msg.TopicPartition)
}

func (c *KafkaComsumer) decompressPayload(payload []byte, compressionType compressor.CompressionType) ([]byte, error) {
	msgCompressor, found := c.compressorsMap[compressionType]
	if !found {
		newCompressor, err := compressor.NewCompressor(compressionType)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		c.compressorsMap[compressionType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	return decompressedBytes, nil
}

func (c *KafkaComsumer) lookupHeaderValue(message *kafka.Message, headerKey string) ([]byte, bool) {
	for _, header := range message.Headers {
		if header.Key == headerKey {
			return header.Value, true
		}
	}
	return nil, false
}

func (c *KafkaComsumer) GetGenericBundleChan() chan *bundle.GenericBundle {
	return c.genericBundlesChan
}
