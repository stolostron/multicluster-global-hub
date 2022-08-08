package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statistics"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/hub-of-hubs/pkg/compressor"
	kafkaclient "github.com/stolostron/hub-of-hubs/pkg/kafka"
	"github.com/stolostron/hub-of-hubs/pkg/kafka/headers"
	kafkaconsumer "github.com/stolostron/hub-of-hubs/pkg/kafka/kafka-consumer"
)

const (
	msgIDTokensLength      = 2
	defaultCompressionType = compressor.NoOp
)

var errMessageIDWrongFormat = errors.New("message ID format is bad")

type KafkaConsumerConfig struct {
	ConsumerID    string
	ConsumerTopic string
}

// NewConsumer creates a new instance of Consumer.
func NewConsumer(committerInterval time.Duration, bootstrapServer, SslCa string, consumerConfig *KafkaConsumerConfig,
	conflationManager *conflator.ConflationManager, statistics *statistics.Statistics, log logr.Logger,
) (*Consumer, error) {
	kafkaConfigMap := &kafka.ConfigMap{
		"bootstrap.servers":       bootstrapServer,
		"client.id":               consumerConfig.ConsumerID,
		"group.id":                consumerConfig.ConsumerID,
		"auto.offset.reset":       "earliest",
		"enable.auto.commit":      "false",
		"socket.keepalive.enable": "true",
		"log.connection.close":    "false", // silence spontaneous disconnection logs, kafka recovers by itself.
	}

	if err := loadSslToConfigMap(SslCa, kafkaConfigMap); err != nil {
		return nil, err
	}

	msgChan := make(chan *kafka.Message)
	kafkaConsumer, err := kafkaconsumer.NewKafkaConsumer(kafkaConfigMap, msgChan, log)
	if err != nil {
		close(msgChan)
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	if err := kafkaConsumer.Subscribe(consumerConfig.ConsumerTopic); err != nil {
		close(msgChan)
		kafkaConsumer.Close()

		return nil, fmt.Errorf("failed to subscribe to requested topic - %v: %w", consumerConfig.ConsumerTopic, err)
	}

	// create committer
	committer, err := newCommitter(committerInterval, consumerConfig.ConsumerTopic, kafkaConsumer,
		conflationManager.GetBundlesMetadata, log)
	if err != nil {
		close(msgChan)
		kafkaConsumer.Close()

		return nil, fmt.Errorf("failed to create committer: %w", err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())

	return &Consumer{
		log:                    log,
		kafkaConsumer:          kafkaConsumer,
		committer:              committer,
		compressorsMap:         make(map[compressor.CompressionType]compressor.Compressor),
		conflationManager:      conflationManager,
		statistics:             statistics,
		msgChan:                msgChan,
		msgIDToRegistrationMap: make(map[string]*transport.BundleRegistration),
		ctx:                    ctx,
		cancelFunc:             cancelFunc,
	}, nil
}

// loadSslToConfigMap loads the given kafka CA to kafka Configmap
func loadSslToConfigMap(SslCa string, kafkaConfigMap *kafka.ConfigMap) error {
	// sslBase64EncodedCertificate
	if SslCa != "" {
		certFileLocation, err := kafkaclient.SetCertificate(&SslCa)
		if err != nil {
			return fmt.Errorf("failed to SetCertificate - %w", err)
		}

		if err = kafkaConfigMap.SetKey("security.protocol", "ssl"); err != nil {
			return fmt.Errorf("failed to SetKey security.protocol - %w", err)
		}

		if err = kafkaConfigMap.SetKey("ssl.ca.location", certFileLocation); err != nil {
			return fmt.Errorf("failed to SetKey ssl.ca.location - %w", err)
		}
	}
	return nil
}

// Consumer abstracts hub-of-hubs/pkg/kafka kafka-consumer's generic usage.
type Consumer struct {
	log               logr.Logger
	kafkaConsumer     *kafkaconsumer.KafkaConsumer
	committer         *committer
	compressorsMap    map[compressor.CompressionType]compressor.Compressor
	conflationManager *conflator.ConflationManager
	statistics        *statistics.Statistics

	msgChan                chan *kafka.Message
	msgIDToRegistrationMap map[string]*transport.BundleRegistration

	ctx        context.Context
	cancelFunc context.CancelFunc
	startOnce  sync.Once
	stopOnce   sync.Once
}

// Start function starts the consumer.
func (c *Consumer) Start() {
	c.startOnce.Do(func() {
		go c.committer.start(c.ctx)
		go c.handleKafkaMessages(c.ctx)
	})
}

// Stop stops the consumer.
func (c *Consumer) Stop() {
	c.stopOnce.Do(func() {
		c.cancelFunc()
		c.kafkaConsumer.Close()
		close(c.msgChan)
	})
}

// Register function registers a msgID to the bundle updates channel.
func (c *Consumer) Register(registration *transport.BundleRegistration) {
	c.msgIDToRegistrationMap[registration.MsgID] = registration
}

func (c *Consumer) handleKafkaMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case msg := <-c.msgChan:
			c.processMessage(msg)
		}
	}
}

func (c *Consumer) processMessage(msg *kafka.Message) {
	compressionType := defaultCompressionType

	if compressionTypeBytes, found := c.lookupHeaderValue(msg, headers.CompressionType); found {
		compressionType = compressor.CompressionType(compressionTypeBytes)
	}

	decompressedPayload, err := c.decompressPayload(msg.Value, compressionType)
	if err != nil {
		c.logError(err, "failed to decompress bundle bytes", msg)
		return
	}

	transportMsg := &Message{}
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

	c.conflationManager.Insert(receivedBundle, newBundleMetadata(msg.TopicPartition.Partition,
		msg.TopicPartition.Offset))
}

func (c *Consumer) logError(err error, errMessage string, msg *kafka.Message) {
	c.log.Error(err, errMessage, "MessageKey", string(msg.Key), "TopicPartition", msg.TopicPartition)
}

func (c *Consumer) lookupHeaderValue(msg *kafka.Message, headerKey string) ([]byte, bool) {
	for _, header := range msg.Headers {
		if header.Key == headerKey {
			return header.Value, true
		}
	}

	return nil, false
}

func (c *Consumer) decompressPayload(payload []byte, msgCompressorType compressor.CompressionType) ([]byte, error) {
	msgCompressor, found := c.compressorsMap[msgCompressorType]
	if !found {
		newCompressor, err := compressor.NewCompressor(msgCompressorType)
		if err != nil {
			return nil, fmt.Errorf("failed to create compressor: %w", err)
		}

		msgCompressor = newCompressor
		c.compressorsMap[msgCompressorType] = msgCompressor
	}

	decompressedBytes, err := msgCompressor.Decompress(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress message: %w", err)
	}

	return decompressedBytes, nil
}
