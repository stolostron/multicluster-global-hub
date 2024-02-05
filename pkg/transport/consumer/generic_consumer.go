// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata/status"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

type GenericConsumer struct {
	log                  logr.Logger
	client               cloudevents.Client
	assembler            *messageAssembler
	messageChan          chan *transport.Message
	eventChan            chan cloudevents.Event
	withDatabasePosition bool
	consumeTopics        []string
}

type GenericConsumeOption func(*GenericConsumer) error

func WithDatabasePosition(fromPosition bool) GenericConsumeOption {
	return func(c *GenericConsumer) error {
		c.withDatabasePosition = fromPosition
		return nil
	}
}

func NewGenericConsumer(tranConfig *transport.TransportConfig, topics []string,
	opts ...GenericConsumeOption) (*GenericConsumer, error) {
	log := ctrl.Log.WithName(fmt.Sprintf("%s-consumer", tranConfig.TransportType))
	var receiver interface{}
	var err error
	switch tranConfig.TransportType {
	case string(transport.Kafka):
		log.Info("transport consumer with cloudevents-kafka receiver")
		receiver, err = getConfluentReceiverProtocol(tranConfig, topics)
		if err != nil {
			return nil, err
		}
	case string(transport.Chan):
		log.Info("transport consumer with go chan receiver")
		if tranConfig.Extends == nil {
			tranConfig.Extends = make(map[string]interface{})
		}
		if _, found := tranConfig.Extends[string(transport.Chan)]; !found {
			tranConfig.Extends[string(transport.Chan)] = gochan.New()
		}
		receiver = tranConfig.Extends[string(transport.Chan)]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", tranConfig.TransportType)
	}

	client, err := cloudevents.NewClient(receiver, client.WithPollGoroutines(1))
	if err != nil {
		return nil, err
	}

	c := &GenericConsumer{
		log:                  log,
		client:               client,
		messageChan:          make(chan *transport.Message),
		eventChan:            make(chan cloudevents.Event),
		assembler:            newMessageAssembler(),
		withDatabasePosition: false,
		consumeTopics:        topics,
	}
	if err := c.applyOptions(opts...); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *GenericConsumer) applyOptions(opts ...GenericConsumeOption) error {
	for _, fn := range opts {
		if err := fn(c); err != nil {
			return err
		}
	}
	return nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	receiveContext := ctx
	if c.withDatabasePosition {
		offsets, err := getInitOffset()
		if err != nil {
			return err
		}
		c.log.Info("init consumer", "offsets", offsets)
		if len(offsets) > 0 {
			receiveContext = kafka_confluent.CommitOffsetCtx(ctx, offsets)
		}
	}

	err := c.client.StartReceiver(receiveContext, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
		c.log.V(2).Info("received message and forward to bundle channel", "event.ID", event.ID())

		topic, ok := event.Extensions()[kafka_confluent.KafkaTopicKey]
		// topic not exist(e2e) or event topic
		if !ok || topic == transport.GenericEventTopic {
			c.eventChan <- event
			return ceprotocol.ResultACK
		}

		transportMessage := &transport.Message{}
		transportMessage.Key = event.ID()
		transportMessage.MsgType = event.Type()
		transportMessage.Destination = event.Source()
		transportMessage.BundleStatus = status.NewThresholdBundleStatus(3, event)

		chunk, isChunk := c.assembler.messageChunk(event)
		if !isChunk {
			transportMessage.Payload = event.Data()
			c.messageChan <- transportMessage
		}
		if payload := c.assembler.assemble(chunk); payload != nil {
			transportMessage.Payload = payload
			c.messageChan <- transportMessage
		}
		return ceprotocol.ResultACK
	})
	if err != nil {
		return fmt.Errorf("failed to start Receiver: %w", err)
	}
	c.log.Info("receiver stopped\n")
	return nil
}

func (c *GenericConsumer) MessageChan() chan *transport.Message {
	return c.messageChan
}

func (c *GenericConsumer) EventChan() chan cloudevents.Event {
	return c.eventChan
}

func getInitOffset() ([]kafka.TopicPartition, error) {
	db := database.GetGorm()
	var positions []models.Transport
	err := db.Where("name ~ ?", "^status*").Find(&positions).Error
	if err != nil {
		return nil, err
	}
	offsetToStart := []kafka.TopicPartition{}
	for i, pos := range positions {
		var kafkaPosition metadata.TransportPosition
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

func getConfluentReceiverProtocol(transportConfig *transport.TransportConfig, topics []string) (interface{}, error) {
	configMap, err := config.GetConfluentConfigMap(transportConfig.KafkaConfig, false)
	if err != nil {
		return nil, err
	}

	return kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithReceiverTopics(topics))
}
