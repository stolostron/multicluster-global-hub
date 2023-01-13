// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

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

type GenericProducer struct {
	log              logr.Logger
	client           cloudevents.Client
	messageSizeLimit int // message size limit in bytes
}

func NewGenericProducer(transportConfig *transport.TransportConfig) (*GenericProducer, error) {
	var sender interface{}
	switch transportConfig.TransportType {
	case string(transport.Kafka):
		var err error
		sender, err = protocol.NewKafkaSender(transportConfig.KafkaConfig)
		if err != nil {
			return nil, err
		}
	case string(transport.Chan): // this go chan protocol is only use for test
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[string(transport.Chan)]; !found {
			transportConfig.Extends[string(transport.Chan)] = gochan.New()
		}
		sender = transportConfig.Extends[string(transport.Chan)]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	client, err := cloudevents.NewClient(sender, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		return nil, err
	}

	return &GenericProducer{
		log:              ctrl.Log.WithName(fmt.Sprintf("%s-producer", transportConfig.TransportType)),
		client:           client,
		messageSizeLimit: transportConfig.KafkaConfig.ProducerConfig.MsgSizeLimitKB * kiloBytesToBytes,
	}, nil
}

func (p *GenericProducer) Send(ctx context.Context, msg *transport.Message) error {
	// TODO: split the large message to chunk
	if len(msg.Payload) > p.messageSizeLimit {
		return fmt.Errorf("message payload size exceeded the limit of %d byte", p.messageSizeLimit)
	}
	event := cloudevents.NewEvent()
	event.SetSpecVersion(cloudevents.VersionV1)
	event.SetSource("global-hub-manager")
	event.SetID(msg.ID)
	event.SetType(msg.MsgType)
	event.SetExtension("version", msg.Version)
	event.SetExtension("key", msg.Key)
	event.SetExtension("destination", msg.Destination)
	event.SetData(cloudevents.ApplicationJSON, msg.Payload)

	if result := p.client.Send(ctx, event); cloudevents.IsUndelivered(result) {
		return fmt.Errorf("failed to send: %v", result)
	}
	return nil
}
