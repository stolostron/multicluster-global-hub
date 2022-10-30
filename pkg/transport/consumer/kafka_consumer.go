// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/helpers"
)

const pollTimeoutMs = 100

var (
	errHeaderNotFound = errors.New("required message header not found")
	parseFail         = "failed to parse bundle"
)

type KafkaConsumerConfig struct {
	ConsumerID    string
	ConsumerTopic string
}

// Consumer abstracts hub-of-hubs/pkg/kafka kafka-consumer's generic usage.
type KafkaConsumer struct {
	log            logr.Logger
	consumer       *kafka.Consumer
	compressorsMap map[compressor.CompressionType]compressor.Compressor
	topic          string

	// messageChan get the message from kafka and put it to the genericBundleChan
	messageChan      chan *kafka.Message
	messageAssembler *kafkaMessageAssembler

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once

	// Used by agent
	leafHubName                     string
	genericBundlesChan              chan *bundle.GenericBundle
	customBundleIDToRegistrationMap map[string]*registration.CustomBundleRegistration
	stopChan                        chan struct{}
	partitionToOffsetToCommitMap    map[int32]kafka.Offset // size limited at all times (low)

	// Used by manager
	committer                  *committer
	conflationManager          *conflator.ConflationManager
	statistics                 *statistics.Statistics
	messageIDToRegistrationMap map[string]*registration.BundleRegistration
}

// NewConsumer creates a new instance of Consumer.
func NewKafkaConsumer(bootstrapServer, sslCA string, consumerConfig *KafkaConsumerConfig, log logr.Logger,
) (*KafkaConsumer, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       bootstrapServer,
		"client.id":               consumerConfig.ConsumerID,
		"group.id":                consumerConfig.ConsumerID,
		"auto.offset.reset":       "earliest",
		"enable.auto.commit":      "false",
		"socket.keepalive.enable": "true",
	}
	err := helpers.LoadSslToConfigMap(sslCA, kafkaConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to configure kafka-consumer - %w", err)
	}

	consumer, err := kafka.NewConsumer(kafkaConfigMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	messageChan := make(chan *kafka.Message)

	kafkaConsumer := &KafkaConsumer{
		log:              log,
		consumer:         consumer,
		messageAssembler: newKafkaMessageAssembler(),
		stopChan:         make(chan struct{}, 1),
		compressorsMap:   make(map[compressor.CompressionType]compressor.Compressor),
		topic:            consumerConfig.ConsumerTopic,
		messageChan:      messageChan,
		ctx:              ctx,
		cancelFunc:       cancelFunc,

		genericBundlesChan:         make(chan *bundle.GenericBundle),
		messageIDToRegistrationMap: make(map[string]*registration.BundleRegistration),

		customBundleIDToRegistrationMap: make(map[string]*registration.CustomBundleRegistration),
		partitionToOffsetToCommitMap:    make(map[int32]kafka.Offset),
	}

	if err := kafkaConsumer.Subscribe(consumerConfig.ConsumerTopic); err != nil {
		close(messageChan)
		kafkaConsumer.log.Info(kafkaConsumer.consumer.Close().Error())
		return nil, fmt.Errorf("failed to subscribe to requested topic - %v: %w", consumerConfig.ConsumerTopic, err)
	}

	return kafkaConsumer, nil
}

func (c *KafkaConsumer) SetCommitter(committer *committer) {
	c.committer = committer
}

func (c *KafkaConsumer) SetStatistics(statistics *statistics.Statistics) {
	c.statistics = statistics
}

func (c *KafkaConsumer) SetLeafHubName(leafHubName string) {
	c.leafHubName = leafHubName
}

func (c *KafkaConsumer) SetConflationManager(conflationMgr *conflator.ConflationManager) {
	c.conflationManager = conflationMgr
}

// Start function starts the consumer.
func (c *KafkaConsumer) Start() {
	c.startOnce.Do(func() {
		if c.committer != nil {
			go c.committer.start(c.ctx)
		}
		go c.handleKafkaMessages(c.ctx)
	})
}

// Stop stops the consumer.
func (c *KafkaConsumer) Stop() {
	c.stopOnce.Do(func() {
		close(c.messageChan)
		close(c.genericBundlesChan)
		close(c.stopChan)
		c.cancelFunc()
		c.log.Info(c.consumer.Close().Error())
	})
}

// Register function registers a bundle ID to a CustomBundleRegistration.
func (c *KafkaConsumer) CustomBundleRegister(msgID string,
	customBundleRegistration *registration.CustomBundleRegistration,
) {
	c.customBundleIDToRegistrationMap[msgID] = customBundleRegistration
}

// Register function registers a msgID to the bundle updates channel.
func (c *KafkaConsumer) BundleRegister(registration *registration.BundleRegistration) {
	c.messageIDToRegistrationMap[registration.MsgID] = registration
}

func (c *KafkaConsumer) SendAsync(msg *transport.Message) {
	// do nothing
}

func (c *KafkaConsumer) handleKafkaMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-c.messageChan:
			c.log.Info("received message and forward to bundle chan...")
			if c.conflationManager == nil {
				c.processMessage(msg)
			} else {
				c.processMessageWithConflation(msg)
			}
		}
	}
}

func (c *KafkaConsumer) processMessageWithConflation(message *kafka.Message) {
	compressionType := compressor.NoOp

	if compressionTypeBytes, found := c.lookupHeaderValue(message, transport.CompressionType); found {
		compressionType = compressor.CompressionType(compressionTypeBytes)
	}

	decompressedPayload, err := c.decompressPayload(message.Value, compressionType)
	if err != nil {
		c.logError(err, "failed to decompress bundle bytes", message)
		return
	}

	transportMessage := &transport.Message{}
	if err := json.Unmarshal(decompressedPayload, transportMessage); err != nil {
		c.logError(err, "failed to parse transport message", message)
		return
	}

	// get msgID
	msgIDTokens := strings.Split(transportMessage.ID, ".") // object id is LH_ID.MSG_ID
	if len(msgIDTokens) != 2 {
		c.logError(errors.New("message ID format is bad"),
			"expecting MessageID of format LH_ID.MSG_ID", message)
		return
	}

	msgID := msgIDTokens[1]
	if _, found := c.messageIDToRegistrationMap[msgID]; !found {
		c.log.Info("no bundle-registration available, not sending bundle", "messageId", transportMessage.ID,
			"messageType", transportMessage.MsgType, "version", transportMessage.Version)
		// no one registered for this msg id
		return
	}

	if !c.messageIDToRegistrationMap[msgID].Predicate() {
		c.log.Info("predicate is false, not sending bundle", "messageId", transportMessage.ID,
			"messageType", transportMessage.MsgType, "version", transportMessage.Version)

		return // bundle-registration predicate is false, do not send the update in the channel
	}

	receivedBundle := c.messageIDToRegistrationMap[msgID].CreateBundleFunc()
	if err := json.Unmarshal(transportMessage.Payload, receivedBundle); err != nil {
		c.logError(err, parseFail, message)
		return
	}

	c.statistics.IncrementNumberOfReceivedBundles(receivedBundle)

	c.conflationManager.Insert(receivedBundle, NewBundleMetadata(message.TopicPartition.Partition,
		message.TopicPartition.Offset))
}

func (c *KafkaConsumer) processMessage(message *kafka.Message) {
	if msgDestinationLeafHubBytes, found :=
		c.lookupHeaderValue(message, transport.DestinationHub); found {
		if string(msgDestinationLeafHubBytes) != c.leafHubName {
			return // if destination is explicitly specified and does not match, drop bundle
		}
	} // if header is not found then assume broadcast

	compressionTypeBytes, found := c.lookupHeaderValue(message, transport.CompressionType)
	if !found {
		c.logError(errors.New("compression type is missing from message description"), "failed to read bundle", message)
		return
	}

	decompressedPayload, err := c.decompressPayload(message.Value,
		compressor.CompressionType(compressionTypeBytes))
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
			c.log.Error(err, parseFail, "MessageID", transportMessage.ID,
				"MessageType", transportMessage.MsgType, "Version", transportMessage.Version)
		}
		return
	}
	// received a custom bundle
	if err := c.SyncCustomBundle(customBundleRegistration, transportMessage.Payload); err != nil {
		c.log.Error(err, parseFail, "MessageID", transportMessage.ID,
			"MessageType", transportMessage.MsgType, "Version", transportMessage.Version)
	}
}

func (c *KafkaConsumer) syncGenericBundle(payload []byte) error {
	receivedBundle := bundle.NewGenericBundle()
	if err := json.Unmarshal(payload, receivedBundle); err != nil {
		return fmt.Errorf("failed to parse bundle - %w", err)
	}
	c.genericBundlesChan <- receivedBundle
	return nil
}

// SyncCustomBundle writes a custom bundle to its respective syncer channel.
func (c *KafkaConsumer) SyncCustomBundle(customBundleRegistration *registration.CustomBundleRegistration,
	payload []byte,
) error {
	receivedBundle := customBundleRegistration.InitBundlesResourceFunc()
	if err := json.Unmarshal(payload, &receivedBundle); err != nil {
		return fmt.Errorf("failed to parse custom bundle - %w", err)
	}
	customBundleRegistration.BundleUpdatesChan <- receivedBundle
	return nil
}

func (c *KafkaConsumer) logError(err error, errMessage string, msg *kafka.Message) {
	c.log.Error(err, errMessage, "MessageKey", string(msg.Key), "TopicPartition", msg.TopicPartition)
}

func (c *KafkaConsumer) decompressPayload(payload []byte, compressionType compressor.CompressionType) ([]byte, error) {
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

func (c *KafkaConsumer) lookupHeaderValue(message *kafka.Message, headerKey string) ([]byte, bool) {
	for _, header := range message.Headers {
		if header.Key == headerKey {
			return header.Value, true
		}
	}
	return nil, false
}

func (c *KafkaConsumer) GetGenericBundleChan() chan *bundle.GenericBundle {
	return c.genericBundlesChan
}

// Consumer returns the wrapped Confluent KafkaConsumer.
func (c *KafkaConsumer) Consumer() *kafka.Consumer {
	return c.consumer
}

// Subscribe subscribes consumer to the given topic.
func (c *KafkaConsumer) Subscribe(topic string) error {
	if err := c.consumer.SubscribeTopics([]string{topic}, nil); err != nil {
		return fmt.Errorf("failed to subscribe to topic - %w", err)
	}

	c.log.Info("started listening", "topic", topic)

	go func() {
		for {
			select {
			case <-c.stopChan:
				_ = c.consumer.Unsubscribe()
				c.log.Info("stopped listening", "topic", topic)
				return
			default:
				c.readMessage()
			}
		}
	}()

	return nil
}

func (c *KafkaConsumer) readMessage() {
	msg, err := c.consumer.ReadMessage(pollTimeoutMs)
	if err != nil && err.(kafka.Error).Code() != kafka.ErrTimedOut {
		c.log.Error(err, "failed to read message")
		return
	}

	if msg == nil {
		return
	}
	fragment, msgIsFragment := c.messageIsFragment(msg)
	if !msgIsFragment {
		// fix offset in-case msg landed on a partition with open fragment collections
		c.messageAssembler.fixMessageOffset(msg)
		c.messageChan <- msg

		return
	}
	// wrap in fragment-info
	fragInfo, err := c.createFragmentInfo(msg, fragment)
	if err != nil {
		c.log.Error(err, "failed to read message", "topic", msg.TopicPartition.Topic)
		return
	}

	if assembledMessage := c.messageAssembler.processFragmentInfo(fragInfo); assembledMessage != nil {
		c.messageChan <- assembledMessage
	}
}

// Commit commits a kafka message.
func (c *KafkaConsumer) Commit(msg *kafka.Message) error {
	if _, err := c.consumer.CommitMessage(msg); err != nil {
		return fmt.Errorf("failed to commit - %w", err)
	}

	return nil
}

func (c *KafkaConsumer) messageIsFragment(msg *kafka.Message) (*messageFragment, bool) {
	offsetHeader, offsetFound := c.lookupHeader(msg, transport.Offset)
	_, sizeFound := c.lookupHeader(msg, transport.Size)

	if !(offsetFound && sizeFound) {
		return nil, false
	}

	return &messageFragment{
		offset: binary.BigEndian.Uint32(offsetHeader.Value),
		bytes:  msg.Value,
	}, true
}

func (c *KafkaConsumer) lookupHeader(msg *kafka.Message, headerKey string) (*kafka.Header, bool) {
	for _, header := range msg.Headers {
		if header.Key == headerKey {
			return &header, true
		}
	}

	return nil, false
}

func (c *KafkaConsumer) createFragmentInfo(msg *kafka.Message, fragment *messageFragment,
) (*messageFragmentInfo, error) {
	timestampHeader, found := c.lookupHeader(msg, transport.FragmentationTimestamp)
	if !found {
		return nil, fmt.Errorf("%w : header key - %s", errHeaderNotFound, transport.FragmentationTimestamp)
	}

	sizeHeader, found := c.lookupHeader(msg, transport.Size)
	if !found {
		return nil, fmt.Errorf("%w : header key - %s", errHeaderNotFound, transport.Size)
	}

	timestamp, err := time.Parse(time.RFC3339, string(timestampHeader.Value))
	if err != nil {
		return nil, fmt.Errorf("header (%s) illegal value - %w",
			transport.FragmentationTimestamp, err)
	}

	size := binary.BigEndian.Uint32(sizeHeader.Value)

	return &messageFragmentInfo{
		key:                    string(msg.Key),
		totalSize:              size,
		fragmentationTimestamp: timestamp,
		fragment:               fragment,
		kafkaMessage:           msg,
	}, nil
}
