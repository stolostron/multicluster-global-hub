// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/utils"
)

type GenericConsumer struct {
	assembler            *messageAssembler
	eventChan            chan *cloudevents.Event
	enableDatabaseOffset bool

	consumerCtx    context.Context
	consumerCancel context.CancelFunc
	client         cloudevents.Client
	kafkaConsumer  *kafka.Consumer

	mutex sync.Mutex

	// topicMetadataRefreshInterval reflects the topic.metadata.refresh.interval.ms and
	// metadata.max.age.ms settings in the consumer config.
	// the default value is 5 mins in Kafka, we set it to 1 min in order to quickly
	// respond to topic changes for the global hub manager only.
	// the global hub manager consumes from topics created dynamically when importing the managed cluster.
	topicMetadataRefreshInterval int
}

type GenericConsumeOption func(*GenericConsumer) error

func EnableDatabaseOffset(enableOffset bool) GenericConsumeOption {
	return func(c *GenericConsumer) error {
		c.enableDatabaseOffset = enableOffset
		return nil
	}
}

func SetTopicMetadataRefreshInterval(interval int) GenericConsumeOption {
	return func(c *GenericConsumer) error {
		c.topicMetadataRefreshInterval = interval
		return nil
	}
}

func NewGenericConsumer(tranConfig *transport.TransportInternalConfig, topics []string,
	opts ...GenericConsumeOption,
) (*GenericConsumer, error) {
	c := &GenericConsumer{
		eventChan:            make(chan *cloudevents.Event),
		assembler:            newMessageAssembler(),
		enableDatabaseOffset: tranConfig.EnableDatabaseOffset,
	}
	// Apply options BEFORE initializing client
	if err := c.applyOptions(opts...); err != nil {
		return nil, err
	}
	if err := c.initClient(tranConfig, topics); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *GenericConsumer) KafkaConsumer() *kafka.Consumer {
	return c.kafkaConsumer
}

// initClient will init the consumer identity, clientProtocol, client
func (c *GenericConsumer) initClient(tranConfig *transport.TransportInternalConfig, topics []string) error {
	var err error
	var clientProtocol interface{}

	switch tranConfig.TransportType {
	case string(transport.Kafka):
		log.Info("transport consumer with cloudevents-kafka receiver")
		c.kafkaConsumer, clientProtocol, err = getConfluentReceiverProtocol(tranConfig,
			topics, c.topicMetadataRefreshInterval)
		if err != nil {
			return err
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
		return fmt.Errorf("transport-type - %s is not a valid option", tranConfig.TransportType)
	}

	c.client, err = cloudevents.NewClient(clientProtocol, client.WithPollGoroutines(1))
	if err != nil {
		return err
	}

	return nil
}

func (c *GenericConsumer) applyOptions(opts ...GenericConsumeOption) error {
	for _, fn := range opts {
		if err := fn(c); err != nil {
			return err
		}
	}
	return nil
}

func (c *GenericConsumer) Reconnect(ctx context.Context,
	tranConfig *transport.TransportInternalConfig, topics []string,
) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := c.initClient(tranConfig, topics)
	if err != nil {
		return err
	}

	// close the previous consumer
	if c.consumerCancel != nil {
		c.consumerCancel()
	}
	c.consumerCtx, c.consumerCancel = context.WithCancel(ctx)
	consumerGroupId := tranConfig.KafkaCredential.ConsumerGroupID
	go func() {
		log.Infof("reconnect consumer: %s", consumerGroupId)
		if err := c.Start(c.consumerCtx); err != nil {
			log.Warnf("stop the consumer(%s): %v", consumerGroupId, err)
		}
		log.Infof("consumer stopped: %s", consumerGroupId)
	}()
	return nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	receiveContext := cectx.WithLogger(ctx, logger.ZapLogger("cloudevents"))
	if c.enableDatabaseOffset {
		offsets, err := getInitOffset(config.GetKafkaOwnerIdentity())
		if err != nil {
			return err
		}
		log.Infow("init consumer with database offsets", "offsets", offsets)
		if len(offsets) > 0 {
			receiveContext = kafka_confluent.WithTopicPartitionOffsets(ctx, offsets)
		}
	}
	// each time the consumer starts, it will only log the first message
	receivedMessage := false
	c.consumerCtx, c.consumerCancel = context.WithCancel(receiveContext)
	err := c.client.StartReceiver(c.consumerCtx, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
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
	for i, pos := range positions {
		var kafkaPosition transport.EventPosition
		err := json.Unmarshal(pos.Payload, &kafkaPosition)
		if err != nil {
			return nil, err
		}
		offsetToStart = append(offsetToStart, kafka.TopicPartition{
			Topic:     &positions[i].Name,
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
