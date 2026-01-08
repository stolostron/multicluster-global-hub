// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/utils"
)

type GenericConsumer struct {
	transportConfigChan  chan *transport.TransportInternalConfig
	enableDatabaseOffset bool
	isManager            bool

	// internal variables
	eventChan chan *cloudevents.Event
	errChan   chan error
	assembler *messageAssembler

	// cached transport for reconnection
	transportConfig *transport.TransportInternalConfig

	mutex sync.Mutex

	// topicMetadataRefreshInterval reflects the topic.metadata.refresh.interval.ms and
	// metadata.max.age.ms settings in the consumer config.
	// the default value is 5 mins in Kafka, we set it to 1 min in order to quickly
	// respond to topic changes for the global hub manager only.
	// the global hub manager consumes from topics created dynamically when importing the managed cluster.
	topicMetadataRefreshInterval int
}

func NewGenericConsumer(transportConfigChan chan *transport.TransportInternalConfig,
	isManager bool, enableDatabaseOffset bool,
) (*GenericConsumer, error) {
	c := &GenericConsumer{
		transportConfigChan:  transportConfigChan,
		isManager:            isManager,
		enableDatabaseOffset: enableDatabaseOffset,

		eventChan: make(chan *cloudevents.Event),
		errChan:   make(chan error),
		assembler: newMessageAssembler(),
	}
	if isManager {
		c.topicMetadataRefreshInterval = constants.TopicMetadataRefreshInterval
	}
	return c, nil
}

// Start runs the consumer loop independently from the reconcile loop.
// It reconnects when receiving updated transport config via the transportConfigChan.
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

		case tranConfig := <-c.transportConfigChan:
			// 1. cancel the previous consumer receiver if exists
			if consumerCancel != nil {
				consumerCancel()
			}

			// 2. init the cloudevents client with the new transport config
			client, err := c.initClient(tranConfig)
			if err != nil {
				log.Errorf("failed to init the client: %v", err)
				return err
			}

			// 3. cache the transport config for potential reconnection on error
			c.transportConfig = tranConfig

			// 4. create new context and start receiving events in a goroutine
			consumerCtx, consumerCancel = context.WithCancel(ctx)
			go func() {
				consumerGroupId := tranConfig.KafkaCredential.ConsumerGroupID
				log.Infof("start receiving events: %s", consumerGroupId)
				if err := c.receive(client, consumerCtx); err != nil {
					log.Warnf("receiver stopped with error(%s): %v", consumerGroupId, err)
					c.errChan <- err
				}
				log.Infof("stop receiving events: %s", consumerGroupId)
			}()

		case <-c.errChan:
			// receiver encountered an error, trigger reconnection with cached config
			if c.transportConfig != nil {
				c.transportConfigChan <- c.transportConfig
			} else {
				log.Warnf("no transport config cached, retry in 5 seconds")
				time.Sleep(5 * time.Second)
			}
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
