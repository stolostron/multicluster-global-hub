// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"fmt"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

const (
	MaxMessageKBLimit    = 1024
	DefaultMessageKBSize = 960
)

type GenericProducer struct {
	log              logr.Logger
	client           cloudevents.Client
	messageSizeLimit int
}

func NewGenericProducer(transportConfig *transport.TransportConfig, defaultTopic string) (*GenericProducer, error) {
	var sender interface{}
	var err error
	messageSize := DefaultMessageKBSize * 1000
	log := ctrl.Log.WithName(fmt.Sprintf("%s-producer", transportConfig.TransportType))

	switch transportConfig.TransportType {
	case string(transport.Kafka):
		if transportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB > 0 {
			messageSize = transportConfig.KafkaConfig.ProducerConfig.MessageSizeLimitKB * 1000
		}
		kafkaProtocol, err := getConfluentSenderProtocol(transportConfig, defaultTopic)
		if err != nil {
			return nil, err
		}

		eventChan, err := kafkaProtocol.Events()
		if err != nil {
			return nil, err
		}
		handleProducerEvents(log, eventChan)
		sender = kafkaProtocol
	case string(transport.Chan): // this go chan protocol is only use for test
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[defaultTopic]; !found {
			transportConfig.Extends[defaultTopic] = gochan.New()
		}
		sender = transportConfig.Extends[defaultTopic]
	default:
		return nil, fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	client, err := cloudevents.NewClient(sender, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		return nil, err
	}

	return &GenericProducer{
		log:              log,
		client:           client,
		messageSizeLimit: messageSize,
	}, nil
}

func (p *GenericProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	// message key
	evtCtx := ctx
	if kafka_confluent.MessageKeyFrom(ctx) == "" {
		evtCtx = kafka_confluent.WithMessageKey(ctx, evt.Type())
	}

	// data
	payloadBytes := evt.Data()
	chunks := p.splitPayloadIntoChunks(payloadBytes)
	if len(chunks) == 1 {
		if ret := p.client.Send(evtCtx, evt); cloudevents.IsUndelivered(ret) {
			return fmt.Errorf("failed to send event to transport: %v", ret)
		}
		return nil
	}

	chunkOffset := 0
	for _, chunk := range chunks {
		evt.SetExtension(transport.ChunkSizeKey, len(payloadBytes))
		chunkOffset += len(chunk)
		evt.SetExtension(transport.ChunkOffsetKey, chunkOffset)
		if err := evt.SetData(cloudevents.ApplicationJSON, chunk); err != nil {
			return fmt.Errorf("failed to set cloudevents data: %v", evt)
		}
		if result := p.client.Send(evtCtx, evt); cloudevents.IsUndelivered(result) {
			return fmt.Errorf("failed to send events to transport: %v", result)
		}
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

func (p *GenericProducer) SetDataLimit(size int) {
	p.messageSizeLimit = size
}

func getSaramaSenderProtocol(transportConfig *transport.TransportConfig, defaultTopic string) (interface{}, error) {
	saramaConfig, err := config.GetSaramaConfig(transportConfig.KafkaConfig)
	if err != nil {
		return nil, err
	}
	// set max message bytes to 1 MB: 1000 000 > config.ProducerConfig.MessageSizeLimitKB * 1000
	saramaConfig.Producer.MaxMessageBytes = MaxMessageKBLimit * 1000
	saramaConfig.Producer.Return.Successes = true
	sender, err := kafka_sarama.NewSender([]string{transportConfig.KafkaConfig.BootstrapServer},
		saramaConfig, defaultTopic)
	if err != nil {
		return nil, err
	}
	return sender, nil
}

func getConfluentSenderProtocol(transportConfig *transport.TransportConfig,
	defaultTopic string,
) (*kafka_confluent.Protocol, error) {
	configMap, err := config.GetConfluentConfigMap(transportConfig.KafkaConfig, true)
	if err != nil {
		return nil, err
	}
	return kafka_confluent.New(kafka_confluent.WithConfigMap(configMap), kafka_confluent.WithSenderTopic(defaultTopic))
}

func handleProducerEvents(log logr.Logger, eventChan chan kafka.Event) {
	// Listen to all the events on the default events channel
	// It's important to read these events otherwise the events channel will eventually fill up
	go func() {
		for e := range eventChan {
			switch ev := e.(type) {
			case *kafka.Message:
				// The message delivery report, indicating success or
				// permanent failure after retries have been exhausted.
				// Application level retries won't help since the client
				// is already configured to do that.
				m := ev
				if m.TopicPartition.Error != nil {
					log.Info("Delivery failed", "error", m.TopicPartition.Error)
				}
			case kafka.Error:
				// Generic client instance-level errors, such as
				// broker connection failures, authentication issues, etc.
				//
				// These errors should generally be considered informational
				// as the underlying client will automatically try to
				// recover from any errors encountered, the application
				// does not need to take action on them.
				log.Info("Transport producer client error, ignore it for most cases", "error", ev)
			}
		}
	}()
}
