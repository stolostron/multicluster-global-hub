// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Shopify/sarama"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
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
	messageSizeLimit int
}

func NewGenericProducer(transportConfig *transport.TransportConfig) (transport.Producer, error) {
	var sender interface{}
	messageSize := 960 * 1000
	switch transportConfig.TransportType {
	case string(transport.Kafka):
		var err error
		sender, err = protocol.NewKafkaSender(transportConfig.KafkaConfig)
		if err != nil {
			return nil, err
		}
		messageSize = transportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB * 1000
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
		messageSizeLimit: messageSize,
	}, nil
}

func (p *GenericProducer) Send(ctx context.Context, msg *transport.Message) error {
	event := cloudevents.NewEvent()
	event.SetSpecVersion(cloudevents.VersionV1)
	event.SetSource("global-hub-manager")
	event.SetID(msg.ID)
	event.SetType(msg.MsgType)
	event.SetTime(time.Now())
	messageBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message to bytes: %s", messageBytes)
	}

	chunks := p.splitPayloadIntoChunks(messageBytes)
	for index, chunk := range chunks {
		event.SetExtension(transport.Size, len(messageBytes))
		event.SetExtension(transport.Offset, index*p.messageSizeLimit)
		if err := event.SetData(cloudevents.ApplicationJSON, chunk); err != nil {
			return fmt.Errorf("failed to set cloudevents data: %v", msg)
		}
		if result := p.client.Send(kafka_sarama.WithMessageKey(ctx, sarama.StringEncoder(event.ID())),
			event); cloudevents.IsUndelivered(result) {
			return fmt.Errorf("failed to send generic message to transport: %s", result.Error())
		}

		p.log.Info("sent message", "ID", msg.ID, "Version", msg.Version)
	}
	return nil
}

func (p *GenericProducer) splitPayloadIntoChunks(payload []byte) [][]byte {
	var chunk []byte
	chunks := make([][]byte, 0, len(payload)/(p.messageSizeLimit)+1)
	for len(payload) >= p.messageSizeLimit {
		chunk, payload = payload[:p.messageSizeLimit], payload[p.messageSizeLimit:]
		chunks = append(chunks, chunk)
	}
	if len(payload) > 0 {
		chunks = append(chunks, payload)
	}
	return chunks
}
