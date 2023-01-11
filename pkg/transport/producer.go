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

const MaxMessageSize = 1024

type Producer interface {
	Send(ctx context.Context, message *Message) error
	// TODO: Do we need the stop method to shut down the protocol(kafka)
}

type GenericProducer struct {
	log    logr.Logger
	client cloudevents.Client
}

func NewGenericProducer(transportConfig *TransportConfig) (*GenericProducer, error) {
	var sender interface{}
	switch transportConfig.TransportType {
	case string(Kafka):
		var err error
		sender, err = protocol.NewKafkaSender(transportConfig.KafkaConfig)
		if err != nil {
			return nil, err
		}
	case string(GoChan): // this go chan protocol is only use for test
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[string(GoChan)]; !found {
			transportConfig.Extends[string(GoChan)] = gochan.New()
		}
		sender = transportConfig.Extends[string(GoChan)]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	client, err := cloudevents.NewClient(sender, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		return nil, err
	}

	return &GenericProducer{
		log:    ctrl.Log.WithName(fmt.Sprintf("%s-producer", transportConfig.TransportType)),
		client: client,
	}, nil
}

func (p *GenericProducer) Send(ctx context.Context, msg *Message) error {
	chunks := p.splitPayloadIntoChunks(MaxMessageSize, msg.Payload)

	for index, chunk := range chunks {
		event := cloudevents.NewEvent()
		event.SetSpecVersion(cloudevents.VersionV1)
		event.SetSource("global-hub-manager")
		event.SetID(msg.ID)
		event.SetType(msg.MsgType)
		event.SetExtension("version", msg.Version)
		event.SetExtension("key", msg.Key)
		event.SetExtension("destination", msg.Destination)
		event.SetExtension("size", len(chunks))
		event.SetExtension("offset", index+1)
		event.SetData(cloudevents.ApplicationJSON, chunk)

		if result := p.client.Send(ctx, event); cloudevents.IsUndelivered(result) {
			return fmt.Errorf("failed to send: %v", result)
		}
		p.log.Info("send message(): ", "id", event.ID(), "type", event.Type(), "size", len(chunks), "offset", index+1)
	}
	return nil
}

func (p *GenericProducer) splitPayloadIntoChunks(maxMessageSize int, payload []byte) [][]byte {
	chunks := make([][]byte, 0, (len(payload)/maxMessageSize)+1)
	var chunk []byte
	for len(payload) >= maxMessageSize {
		chunk, payload = payload[:maxMessageSize], payload[maxMessageSize:]
		chunks = append(chunks, chunk)
	}
	if len(payload) > 0 {
		chunks = append(chunks, payload)
	}
	return chunks
}
