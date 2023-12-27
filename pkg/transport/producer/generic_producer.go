// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
)

const (
	MaxMessageKBLimit    = 1024
	DefaultMessageKBSize = 960
	BundleVersionKey     = "bundleVersion"
)

type GenericProducer struct {
	log              logr.Logger
	client           cloudevents.Client
	messageSizeLimit int
}

func NewGenericProducer(transportConfig *transport.TransportConfig) (transport.Producer, error) {
	var sender interface{}
	var err error
	messageSize := DefaultMessageKBSize * 1000

	switch transportConfig.TransportType {
	case string(transport.Kafka):
		if transportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > 0 {
			messageSize = transportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB * 1000
		}
		sender, err = getConfluentSenderProtocol(transportConfig)
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
		messageSizeLimit: messageSize,
	}, nil
}

func (p *GenericProducer) Send(ctx context.Context, msg *transport.Message) error {
	event := cloudevents.NewEvent()
	event.SetID(msg.Key)
	event.SetType(msg.MsgType)
	event.SetTime(time.Now())
	event.SetSource(msg.Destination)
	if msg.Destination == "" {
		event.SetSource(transport.Broadcast)
	}

	messageBytes := msg.Payload
	chunks := p.splitPayloadIntoChunks(messageBytes)
	for index, chunk := range chunks {
		event.SetExtension(transport.ChunkSizeKey, len(messageBytes))
		event.SetExtension(transport.ChunkOffsetKey, index*p.messageSizeLimit)
		if err := event.SetData(cloudevents.ApplicationJSON, chunk); err != nil {
			return fmt.Errorf("failed to set cloudevents data: %v", msg)
		}
		if result := p.client.Send(kafka_confluent.WithMessageKey(ctx, msg.Key), event); cloudevents.IsUndelivered(result) {
			return fmt.Errorf("failed to send generic message to transport: %s", result.Error())
		}

		p.log.Info("sent message", "Key", msg.Key)
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

func getSaramaSenderProtocol(transportConfig *transport.TransportConfig) (interface{}, error) {
	saramaConfig, err := config.GetSaramaConfig(transportConfig.KafkaConfig)
	if err != nil {
		return nil, err
	}
	// set max message bytes to 1 MB: 1000 000 > config.ProducerConfig.MessageSizeLimitKB * 1000
	saramaConfig.Producer.MaxMessageBytes = MaxMessageKBLimit * 1000
	saramaConfig.Producer.Return.Successes = true
	sender, err := kafka_sarama.NewSender([]string{transportConfig.KafkaConfig.BootstrapServer},
		saramaConfig, transportConfig.KafkaConfig.ProducerConfig.ProducerTopic)
	if err != nil {
		return nil, err
	}
	return sender, nil
}

func getConfluentSenderProtocol(transportConfig *transport.TransportConfig) (interface{}, error) {
	configMap, err := config.GetConfluentConfigMap(transportConfig.KafkaConfig, true)
	if err != nil {
		return nil, err
	}
	return kafka_confluent.New(kafka_confluent.WithConfigMap(configMap),
		kafka_confluent.WithSenderTopic(transportConfig.KafkaConfig.ProducerConfig.ProducerTopic))
}
