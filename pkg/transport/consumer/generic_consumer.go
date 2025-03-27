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
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

var transportID string

type GenericConsumer struct {
	log                  *zap.SugaredLogger
	assembler            *messageAssembler
	eventChan            chan *cloudevents.Event
	enableDatabaseOffset bool
	clusterID            string

	consumerCtx    context.Context
	consumerCancel context.CancelFunc
	client         cloudevents.Client

	mutex sync.Mutex
}

type GenericConsumeOption func(*GenericConsumer) error

func EnableDatabaseOffset(enableOffset bool) GenericConsumeOption {
	return func(c *GenericConsumer) error {
		c.enableDatabaseOffset = enableOffset
		return nil
	}
}

func NewGenericConsumer(tranConfig *transport.TransportInternalConfig,
	opts ...GenericConsumeOption,
) (*GenericConsumer, error) {
	c := &GenericConsumer{
		log:                  logger.ZapLogger(fmt.Sprintf("%s-consumer", tranConfig.TransportType)),
		eventChan:            make(chan *cloudevents.Event),
		assembler:            newMessageAssembler(),
		enableDatabaseOffset: tranConfig.EnableDatabaseOffset,
	}
	if err := c.initClient(tranConfig); err != nil {
		return nil, err
	}
	if err := c.applyOptions(opts...); err != nil {
		return nil, err
	}
	return c, nil
}

// initClient will init the consumer identity, clientProtocol, client
func (c *GenericConsumer) initClient(tranConfig *transport.TransportInternalConfig) error {
	var err error
	var clientProtocol interface{}

	c.clusterID = tranConfig.KafkaCredential.ClusterID
	topics := []string{tranConfig.KafkaCredential.StatusTopic}
	if !tranConfig.IsManager {
		topics[0] = tranConfig.KafkaCredential.SpecTopic
	}

	switch tranConfig.TransportType {
	case string(transport.Kafka):
		c.log.Info("transport consumer with cloudevents-kafka receiver")
		clientProtocol, err = getConfluentReceiverProtocol(tranConfig, topics)
		if err != nil {
			return err
		}
	case string(transport.Chan):
		c.log.Info("transport consumer with go chan receiver")
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

func (c *GenericConsumer) Reconnect(ctx context.Context, tranConfig *transport.TransportInternalConfig) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	err := c.initClient(tranConfig)
	if err != nil {
		return err
	}

	// close the previous consumer
	if c.consumerCancel != nil {
		c.consumerCancel()
	}
	c.consumerCtx, c.consumerCancel = context.WithCancel(ctx)

	go func() {
		if err := c.Start(c.consumerCtx); err != nil {
			c.log.Error(err, "failed to reconnect(start) the consumer")
		}
	}()
	return nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	receiveContext := cectx.WithLogger(ctx, logger.ZapLogger("cloudevents"))
	if c.enableDatabaseOffset {
		offsets, err := getInitOffset(c.clusterID)
		if err != nil {
			return err
		}
		c.log.Info("init consumer", "offsets", offsets)
		if len(offsets) > 0 {
			receiveContext = kafka_confluent.WithTopicPartitionOffsets(ctx, offsets)
		}
	}

	c.consumerCtx, c.consumerCancel = context.WithCancel(receiveContext)
	err := c.client.StartReceiver(c.consumerCtx, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
		c.log.Debugw("received message", "event.Source", event.Source(), "event.Type", event.Type())

		chunk, isChunk := c.assembler.messageChunk(event)
		c.log.Debugf("chunk: %v", isChunk)
		if !isChunk {
			c.log.Debugf("chunk: %v", isChunk)
			c.eventChan <- &event
			return ceprotocol.ResultACK
		}
		if payload := c.assembler.assemble(chunk); payload != nil {
			c.log.Debugf("payload: %v", payload)
			if err := event.SetData(cloudevents.ApplicationJSON, payload); err != nil {
				c.log.Error(err, "failed the set the assembled data to event")
			} else {
				c.eventChan <- &event
			}
		}
		return ceprotocol.ResultACK
	})
	if err != nil {
		return fmt.Errorf("failed to start Receiver: %w", err)
	}
	c.log.Info("receiver stopped\n")
	return nil
}

func (c *GenericConsumer) EventChan() chan *cloudevents.Event {
	return c.eventChan
}

func getInitOffset(kafkaClusterIdentity string) ([]kafka.TopicPartition, error) {
	db := database.GetGorm()
	var positions []models.Transport
	err := db.Where("name ~ ?", "^status*").
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

func getConfluentReceiverProtocol(transportConfig *transport.TransportInternalConfig, topics []string) (
	interface{}, error,
) {
	configMap, err := config.GetConfluentConfigMapByKafkaCredential(transportConfig.KafkaCredential,
		transportConfig.ConsumerGroupId)
	if err != nil {
		return nil, err
	}

	return kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithReceiverTopics(topics))
}

func TransportID() string {
	return transportID
}
