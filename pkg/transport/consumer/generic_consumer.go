// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package consumer

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	ceprotocol "github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/cloudevents/sdk-go/v2/types"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
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
		assembler:   newMessageAssembler(),
	}, nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	err := c.client.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) ceprotocol.Result {
		c.log.Info("received message and forward to bundle channel")
		fmt.Printf("%s", event)

		chunk, isChunk := c.messageChunk(event)
		if !isChunk {
			transportMessage := &transport.Message{}
			if err := event.DataAs(transportMessage); err != nil {
				c.log.Error(err, "get transport message error", "event.ID", event.ID())
				return ceprotocol.ResultNACK
			}
			c.messageChan <- transportMessage
			return ceprotocol.ResultACK
		}

		if transportMessage := c.assembler.processChunk(chunk); transportMessage != nil {
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

func (c *GenericConsumer) messageChunk(e cloudevents.Event) (*messageChunk, bool) {
	offset, err := types.ToInteger(e.Extensions()[transport.Offset])
	if err != nil {
		c.log.Error(err, "event offset parse error")
		return nil, false
	}

	size, err := types.ToInteger(e.Extensions()[transport.Size])
	if err != nil {
		c.log.Error(err, "event size parse error")
		return nil, false
	}

	return &messageChunk{
		id:        e.ID(),
		timestamp: e.Time(),
		offset:    int(offset),
		size:      int(size),
		bytes:     e.Data(),
	}, true
}
