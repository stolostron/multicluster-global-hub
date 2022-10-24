// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package fake

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/compressor"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/consumer"
)

const (
	msgIDTokensLength      = 2
	defaultCompressionType = compressor.NoOp
)

var errMessageIDWrongFormat = errors.New("message ID format is bad")

type KafkaTestConsumer struct {
	log               logr.Logger
	compressorsMap    map[compressor.CompressionType]compressor.Compressor
	conflationManager *conflator.ConflationManager
	statistics        *statistics.Statistics

	msgChan                chan *kafka.Message
	msgIDToRegistrationMap map[string]*registration.BundleRegistration

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once
}

// NewKafkaTestConsumer creates a new instance of TestConsumer.
func NewKafkaTestConsumer(messageChan chan *kafka.Message,
	conflationManager *conflator.ConflationManager,
	statistics *statistics.Statistics, log logr.Logger,
) (*KafkaTestConsumer, error) {
	ctx, cancelFunc := context.WithCancel(context.Background())

	return &KafkaTestConsumer{
		log:                    log,
		compressorsMap:         make(map[compressor.CompressionType]compressor.Compressor),
		conflationManager:      conflationManager,
		statistics:             statistics,
		msgChan:                messageChan,
		msgIDToRegistrationMap: make(map[string]*registration.BundleRegistration),
		ctx:                    ctx,
		cancelFunc:             cancelFunc,
	}, nil
}

// Start function starts the consumer.
func (c *KafkaTestConsumer) Start() {
	c.startOnce.Do(func() {
		go c.handleKafkaMessages(c.ctx)
	})
}

// Stop stops the consumer.
func (c *KafkaTestConsumer) Stop() {
	c.stopOnce.Do(func() {
		c.cancelFunc()
		close(c.msgChan)
	})
}

// Register function registers a msgID to the bundle updates channel.
func (c *KafkaTestConsumer) BundleRegister(registration *registration.BundleRegistration) {
	c.msgIDToRegistrationMap[registration.MsgID] = registration
}

func (c *KafkaTestConsumer) CustomBundleRegister(msgID string,
	customBundleRegistration *registration.CustomBundleRegistration) {
	// do nothing
}

// SendAsync sends a message to the transport component asynchronously.
func (c *KafkaTestConsumer) SendAsync(msg *transport.Message) {
	// do nothing
}

func (c *KafkaTestConsumer) handleKafkaMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-c.msgChan:
			c.processMessage(msg)
		}
	}
}

func (c *KafkaTestConsumer) processMessage(msg *kafka.Message) {
	compressionType := defaultCompressionType

	if compressionTypeBytes, found := c.lookupHeaderValue(msg, transport.CompressionType); found {
		compressionType = compressor.CompressionType(compressionTypeBytes)
	}

	decompressedPayload, err := c.decompressPayload(msg.Value, compressionType)
	if err != nil {
		c.logError(err, "failed to decompress bundle bytes", msg)
		return
	}

	transportMsg := &transport.Message{}
	if err := json.Unmarshal(decompressedPayload, transportMsg); err != nil {
		c.logError(err, "failed to parse transport message", msg)
		return
	}

	// get msgID
	msgIDTokens := strings.Split(transportMsg.ID, ".") // object id is LH_ID.MSG_ID
	if len(msgIDTokens) != msgIDTokensLength {
		c.logError(errMessageIDWrongFormat, "expecting MessageID of format LH_ID.MSG_ID", msg)
		return
	}

	msgID := msgIDTokens[1]
	if _, found := c.msgIDToRegistrationMap[msgID]; !found {
		c.log.Info("no bundle-registration available, not sending bundle", "messageId", transportMsg.ID,
			"messageType", transportMsg.MsgType, "version", transportMsg.Version)
		// no one registered for this msg id
		return
	}

	if !c.msgIDToRegistrationMap[msgID].Predicate() {
		c.log.Info("predicate is false, not sending bundle", "messageId", transportMsg.ID,
			"messageType", transportMsg.MsgType, "version", transportMsg.Version)

		return // bundle-registration predicate is false, do not send the update in the channel
	}

	receivedBundle := c.msgIDToRegistrationMap[msgID].CreateBundleFunc()
	if err := json.Unmarshal(transportMsg.Payload, receivedBundle); err != nil {
		c.logError(err, "failed to parse bundle", msg)
		return
	}

	c.statistics.IncrementNumberOfReceivedBundles(receivedBundle)

	c.conflationManager.Insert(receivedBundle, consumer.NewBundleMetadata(msg.TopicPartition.Partition,
		msg.TopicPartition.Offset))
}

func (c *KafkaTestConsumer) logError(err error, errMessage string, msg *kafka.Message) {
	c.log.Error(err, errMessage, "MessageKey", string(msg.Key), "TopicPartition", msg.TopicPartition)
}

func (c *KafkaTestConsumer) lookupHeaderValue(msg *kafka.Message, headerKey string) ([]byte, bool) {
	for _, header := range msg.Headers {
		if header.Key == headerKey {
			return header.Value, true
		}
	}

	return nil, false
}

func (c *KafkaTestConsumer) decompressPayload(payload []byte, compressType compressor.CompressionType) ([]byte, error) {
	msgCompressor, found := c.compressorsMap[compressType]
	if !found {
		newCompressor, err := compressor.NewCompressor(compressType)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		c.compressorsMap[compressType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	return decompressedBytes, nil
}
