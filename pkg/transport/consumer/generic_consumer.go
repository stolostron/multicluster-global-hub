// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata/status"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

type GenericConsumer struct {
	log         logr.Logger
	client      cloudevents.Client
	assembler   *messageAssembler
	messageChan chan *transport.Message
}

func NewGenericConsumer(transportConfig *transport.TransportConfig) (*GenericConsumer, error) {
	log := ctrl.Log.WithName(fmt.Sprintf("%s-consumer", transportConfig.TransportType))
	var receiver interface{}
	var err error
	switch transportConfig.TransportType {
	case string(transport.Kafka):
		log.Info("transport consumer with cloudevents-kafka receiver")
		receiver, err = getConfluentReceiverProtocol(transportConfig)
		if err != nil {
			return nil, err
		}
	case string(transport.Chan):
		log.Info("transport consumer with go chan receiver")
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[string(transport.Chan)]; !found {
			transportConfig.Extends[string(transport.Chan)] = gochan.New()
		}
		receiver = transportConfig.Extends[string(transport.Chan)]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	client, err := cloudevents.NewClient(receiver, client.WithPollGoroutines(1))
	if err != nil {
		return nil, err
	}

	return &GenericConsumer{
		log:         log,
		client:      client,
		messageChan: make(chan *transport.Message),
		assembler:   newMessageAssembler(),
	}, nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	err := c.client.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
		c.log.V(2).Info("received message and forward to bundle channel", "event.ID", event.ID())

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
		return ceprotocol.ResultNACK
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

func getConfluentReceiverProtocol(transportConfig *transport.TransportConfig) (interface{}, error) {
	configMap, err := config.GetConfluentConfigMap(transportConfig.KafkaConfig, false)
	if err != nil {
		return nil, err
	}

	return kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithReceiverTopics([]string{transportConfig.KafkaConfig.ConsumerConfig.ConsumerTopic}))
}
