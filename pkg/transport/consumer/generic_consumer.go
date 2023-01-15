// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
)

type GenericConsumer struct {
	log         logr.Logger
	client      cloudevents.Client
	messageChan chan *transport.Message
}

func NewGenericConsumer(transportConfig *transport.TransportConfig) (*GenericConsumer, error) {
	log := ctrl.Log.WithName(fmt.Sprintf("%s-consumer", transportConfig.TransportType))
	var receiver interface{}
	switch transportConfig.TransportType {
	case string(transport.Kafka):
		var err error
		log.Info("transport consumer with kafka receiver")
		receiver, err = protocol.NewKafkaReceiver(transportConfig.KafkaConfig)
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

	client, err := cloudevents.NewClient(receiver)
	if err != nil {
		return nil, err
	}

	return &GenericConsumer{
		log:         log,
		client:      client,
		messageChan: make(chan *transport.Message),
	}, nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	err := c.client.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) {
		// TODO: consumer the large message by chunk?
		c.log.Info("received message and forward to bundle channel")
		fmt.Printf("%s", event)

		transportMessage := &transport.Message{}
		if err := event.DataAs(transportMessage); err != nil {
			c.log.Error(err, "get transport message error", "event.ID", event.ID())
			return
		}
		c.messageChan <- transportMessage
	})
	if err != nil {
		return fmt.Errorf("failed to start Receiver: %w", err)
	}
	c.log.Info("receiver stopped\n")
	return nil
}

func (c *GenericConsumer) AcquireMessage() *transport.Message {
	return <-c.messageChan
}
