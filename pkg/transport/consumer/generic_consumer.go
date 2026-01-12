// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/utils"
)

const (
	defaultStableThreshold = 1 * time.Minute
)

type GenericConsumer struct {
	// transportConfigChan receives transport config to trigger consumer reconnection.
	// This avoids race conditions by passing config explicitly instead of sharing a pointer.
	transportConfigChan  chan *transport.TransportInternalConfig
	enableDatabaseOffset bool
	isManager            bool

	// internal variables
	eventChan chan *cloudevents.Event
	assembler *messageAssembler

	// backoff for reconnection with exponential backoff
	// Note: Only accessed from Start() goroutine, no mutex needed
	backoff wait.Backoff

	// topicMetadataRefreshInterval reflects the topic.metadata.refresh.interval.ms and
	// metadata.max.age.ms settings in the consumer config.
	// the default value is 5 mins in Kafka, we set it to 1 min in order to quickly
	// respond to topic changes for the global hub manager only.
	// the global hub manager consumes from topics created dynamically when importing the managed cluster.
	topicMetadataRefreshInterval int
}

func NewGenericConsumer(isManager bool, enableDatabaseOffset bool) (*GenericConsumer, error) {
	c := &GenericConsumer{
		transportConfigChan:  make(chan *transport.TransportInternalConfig, 1),
		isManager:            isManager,
		enableDatabaseOffset: enableDatabaseOffset,

		eventChan: make(chan *cloudevents.Event),
		assembler: newMessageAssembler(),
		backoff:   newBackoff(),
	}
	if isManager {
		c.topicMetadataRefreshInterval = constants.TopicMetadataRefreshInterval
	}
	return c, nil
}

// newBackoff creates a new backoff configuration
func newBackoff() wait.Backoff {
	return wait.Backoff{
		Duration: 1 * time.Second,
		Factor:   2.0,
		Jitter:   0.1,
		Steps:    math.MaxInt32,
		Cap:      30 * time.Second,
	}
}

// resetBackoff resets the backoff to initial state.
// Note: Not thread-safe. Only call from tests or within getBackoffDuration (which holds the lock).
func (c *GenericConsumer) resetBackoff() {
	c.backoff = newBackoff()
}

// getBackoffDuration returns the current backoff duration using wait.Backoff.Step()
// Note: Only called from Start() goroutine, no synchronization needed.
func (c *GenericConsumer) getBackoffDuration(lastTime time.Time) time.Duration {
	// if connection was stable (ran > threshold), reset backoff
	if time.Since(lastTime) > defaultStableThreshold {
		log.Infof("connection was stable, resetting backoff")
		c.resetBackoff()
	}
	return c.backoff.Step()
}

// Start runs the consumer loop independently from the reconcile loop.
// It reconnects when receiving new transport config via the transportConfigChan.
// Returns error to trigger graceful shutdown if client initialization fails.
func (c *GenericConsumer) Start(ctx context.Context) error {
	var consumerCtx context.Context
	var consumerCancel context.CancelFunc
	for {
		select {
		case <-ctx.Done():
			if consumerCancel != nil {
				consumerCancel()
			}
			log.Infof("context done, stop the consumer")
			return nil

		case transportConfig := <-c.transportConfigChan:
			// 1. cancel the previous consumer receiver if exists
			if consumerCancel != nil {
				consumerCancel()
			}

			// 2. init the cloudevents client with the new transport config
			client, err := c.initClient(transportConfig)
			if err != nil {
				// return error to trigger graceful shutdown via controller-runtime manager
				return fmt.Errorf("failed to init transport client: %w", err)
			}

			// 4. create new context and start receiving events in a goroutine
			consumerCtx, consumerCancel = context.WithCancel(ctx)
			go func(ctx context.Context, config *transport.TransportInternalConfig) {
				consumerGroupId := config.KafkaCredential.ConsumerGroupID
				log.Infof("start receiving events: %s", consumerGroupId)
				startTime := time.Now()
				if err := c.receive(client, ctx); err != nil {
					log.Infof("receiver cancelled, skip reconnect (consumerGroupId=%s)", consumerGroupId)
				}
				log.Infof("stop receiving events: %s", consumerGroupId)

				// only reconnect if receiver exited unexpectedly (not cancelled by new signal)
				if ctx.Err() == context.Canceled {
					log.Infof("receiver cancelled, skip reconnection for the current group id: %s", consumerGroupId)
					return
				}

				// backoff before reconnect to avoid rapid retry loops
				backoff := c.getBackoffDuration(startTime)
				log.Infof("receiver exited unexpectedly, reconnect in %v (consumerGroupId=%s)", backoff, consumerGroupId)
				time.Sleep(backoff)
				// resend the current config to trigger reconnection
				c.transportConfigChan <- config
			}(consumerCtx, transportConfig)
		}
	}
}

// initClient will init the consumer identity, clientProtocol, client
func (c *GenericConsumer) initClient(tranConfig *transport.TransportInternalConfig) (cloudevents.Client, error) {
	topics := []string{tranConfig.KafkaCredential.SpecTopic}
	if c.isManager {
		topics = []string{tranConfig.KafkaCredential.StatusTopic}
	}

	var err error
	var clientProtocol interface{}

	switch tranConfig.TransportType {
	case string(transport.Kafka):
		log.Info("transport consumer with cloudevents-kafka receiver")
		_, clientProtocol, err = getConfluentReceiverProtocol(tranConfig,
			topics, c.topicMetadataRefreshInterval)
		if err != nil {
			return nil, err
		}
	case string(transport.Chan):
		log.Info("transport consumer with go chan receiver")
		if tranConfig.Extends == nil {
			tranConfig.Extends = make(map[string]interface{})
		}
		topic := topics[0]
		if _, found := tranConfig.Extends[topic]; !found {
			tranConfig.Extends[topic] = gochan.New()
		}
		clientProtocol = tranConfig.Extends[topic]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", tranConfig.TransportType)
	}

	client, err := cloudevents.NewClient(clientProtocol, client.WithPollGoroutines(1))
	if err != nil {
		return nil, err
	}

	return client, nil
}

func (c *GenericConsumer) receive(client cloudevents.Client, ctx context.Context) error {
	receiveContext := cectx.WithLogger(ctx, logger.ZapLogger("cloudevents"))
	if c.enableDatabaseOffset {
		offsets, err := getInitOffset(config.GetKafkaOwnerIdentity())
		if err != nil {
			return err
		}
		log.Infow("init consumer with database offsets", "offsets", offsets)
		if len(offsets) > 0 {
			receiveContext = kafka_confluent.WithTopicPartitionOffsets(receiveContext, offsets)
		}
	}
	// each time the consumer starts, it will only log the first message
	receivedMessage := false
	err := client.StartReceiver(receiveContext, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
		log.Debugw("received message", "event.Source", event.Source(), "event.Type", enum.ShortenEventType(event.Type()))

		if !receivedMessage {
			receivedMessage = true
			log.Infow("received message", "topic", event.Extensions()[kafka_confluent.KafkaTopicKey],
				"partition", event.Extensions()[kafka_confluent.KafkaPartitionKey],
				"offset", event.Extensions()[kafka_confluent.KafkaOffsetKey])
		}

		chunk, isChunk := c.assembler.messageChunk(event)
		if !isChunk {
			c.eventChan <- &event
			return ceprotocol.ResultACK
		}
		if payload := c.assembler.assemble(chunk); payload != nil {
			if err := event.SetData(cloudevents.ApplicationJSON, payload); err != nil {
				log.Errorw("failed the set the assembled data to event", "error", err)
			} else {
				c.eventChan <- &event
			}
		}
		return ceprotocol.ResultACK
	})
	if err != nil {
		return fmt.Errorf("consumer receiver stopped with error: %w", err)
	}
	receivedMessage = false
	return nil
}

func (c *GenericConsumer) EventChan() chan *cloudevents.Event {
	return c.eventChan
}

func (c *GenericConsumer) ConfigChan() chan *transport.TransportInternalConfig {
	return c.transportConfigChan
}

func getInitOffset(kafkaClusterIdentity string) ([]kafka.TopicPartition, error) {
	db := database.GetGorm()
	var positions []models.Transport
	// TODO: clean the expired offset, maybe consider to delete the record when detach the managed hub
	err := db.
		Where("payload->>'ownerIdentity' <> ? AND payload->>'ownerIdentity' = ?", "", kafkaClusterIdentity).
		Find(&positions).Error
	if err != nil {
		return nil, err
	}
	offsetToStart := []kafka.TopicPartition{}
	for _, pos := range positions {
		var kafkaPosition transport.EventPosition
		err := json.Unmarshal(pos.Payload, &kafkaPosition)
		if err != nil {
			return nil, err
		}
		// Name is in format "topic@partition", extract the topic part
		topic := pos.Name
		if idx := strings.LastIndex(pos.Name, "@"); idx != -1 {
			topic = pos.Name[:idx]
		}
		offsetToStart = append(offsetToStart, kafka.TopicPartition{
			Topic:     &topic,
			Partition: kafkaPosition.Partition,
			Offset:    kafka.Offset(kafkaPosition.Offset),
		})
	}
	return offsetToStart, nil
}

// func getSaramaReceiverProtocol(transportConfig *transport.TransportConfig) (interface{}, error) {
// 	saramaConfig, err := config.GetSaramaConfig(transportConfig.KafkaConfig)
// 	if err != nil {
// 		return nil, err
// 	}
// 	// if set this to false, it will consume message from beginning when restart the client
// 	saramaConfig.Consumer.Offsets.AutoCommit.Enable = true
// 	saramaConfig.Consumer.Offsets.Initial = sarama.OffsetOldest
// 	// set the consumer groupId = clientId
// 	return kafka_sarama.NewConsumer([]string{transportConfig.KafkaConfig.BootstrapServer}, saramaConfig,
// 		transportConfig.KafkaConfig.ConsumerConfig.ConsumerID,
// 		transportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic)
// }

func getConfluentReceiverProtocol(transportConfig *transport.TransportInternalConfig,
	topics []string, topicMetadataRefreshInterval int) (
	*kafka.Consumer, interface{}, error,
) {
	configMap, err := config.GetConfluentConfigMapByKafkaCredential(transportConfig.KafkaCredential,
		transportConfig.KafkaCredential.ConsumerGroupID, topicMetadataRefreshInterval)
	if err != nil {
		return nil, nil, err
	}
	log.Debugw("the configurations applied to the Kafka consumer", "configMap",
		utils.FilterSensitiveKafkaConfig(configMap))

	consumer, err := kafka.NewConsumer(configMap)
	if err != nil {
		return nil, nil, err
	}

	protocol, err := kafka_confluent.New(kafka_confluent.WithReceiver(consumer),
		kafka_confluent.WithReceiverTopics(topics))
	if err != nil {
		return nil, nil, err
	}
	return consumer, protocol, nil
}
