// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package transport

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport/protocol"
)

type Consumer interface {
	// It will start the underlying receiver protocol as it has been configured. This call is blocking
	Start(ctx context.Context) error
}

type GenericConsumer struct {
	log       logr.Logger
	client    cloudevents.Client
	eventChan chan *cloudevents.Event
}

func NewGenericConsumer(transportConfig *TransportConfig) (*GenericConsumer, error) {
	var receiver interface{}
	switch transportConfig.TransportType {
	case string(Kafka):
		var err error
		receiver, err = protocol.NewKafkaReceiver(transportConfig.KafkaConfig)
		if err != nil {
			return nil, err
		}
	case string(GoChan):
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[string(GoChan)]; !found {
			transportConfig.Extends[string(GoChan)] = gochan.New()
		}
		receiver = transportConfig.Extends[string(GoChan)]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	client, err := cloudevents.NewClient(receiver)
	if err != nil {
		return nil, err
	}

	return &GenericConsumer{
		log:       ctrl.Log.WithName(fmt.Sprintf("%s-consumer", transportConfig.TransportType)),
		client:    client,
		eventChan: make(chan *cloudevents.Event),
	}, nil
}

func (c *GenericConsumer) Start(ctx context.Context) error {
	err := c.client.StartReceiver(ctx, func(ctx context.Context, event cloudevents.Event) {
		// TODO: process message
		fmt.Printf("%s", event)
		c.eventChan <- &event
	})
	// todo close channel and other resource
	if err != nil {
		return fmt.Errorf("failed to start Receiver: %w", err)
	}
	c.log.Info("receiver stopped\n")
	return nil
}

func (c *GenericConsumer) GetMessageChan() chan *cloudevents.Event {
	return c.eventChan
}
