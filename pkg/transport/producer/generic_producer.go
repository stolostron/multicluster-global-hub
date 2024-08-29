// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"fmt"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	"github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/protocol"
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
	clientProtocol   interface{}
	client           cloudevents.Client
	messageSizeLimit int
}

func NewGenericProducer(transportConfig *transport.TransportConfig) (*GenericProducer, error) {
	genericProducer := &GenericProducer{
		log:              ctrl.Log.WithName(fmt.Sprintf("%s-producer", transportConfig.TransportType)),
		messageSizeLimit: DefaultMessageKBSize * 1000,
	}
	err := genericProducer.initClient(transportConfig)
	if err != nil {
		return nil, err
	}

	return genericProducer, nil
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

// Reconnect close the previous producer state and init a new producer
func (p *GenericProducer) Reconnect(config *transport.TransportConfig) error {
	closer, ok := p.clientProtocol.(protocol.Closer)
	if ok {
		if err := closer.Close(context.Background()); err != nil {
			return fmt.Errorf("failed to close the previous producer: %w", err)
		}
	}
	return p.initClient(config)
}

// initClient will init/update the client, clientProtocol and messageLimitSize based on the transportConfig
func (p *GenericProducer) initClient(transportConfig *transport.TransportConfig) error {
	topic := transportConfig.KafkaCredential.SpecTopic
	if !transportConfig.IsManager {
		topic = transportConfig.KafkaCredential.StatusTopic
	}

	if (transportConfig.TransportType == string(transport.Kafka) ||
		transportConfig.TransportType == string(transport.Multiple)) &&
		transportConfig.KafkaCredential != nil {
		kafkaProtocol, err := getConfluentSenderProtocol(transportConfig.KafkaCredential, topic)
		if err != nil {
			return err
		}

		eventChan, err := kafkaProtocol.Events()
		if err != nil {
			return err
		}
		handleProducerEvents(p.log, eventChan)
		p.clientProtocol = kafkaProtocol
	}

	// this go chan protocol is only use for test
	if transportConfig.TransportType == string(transport.Chan) {
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[topic]; !found {
			transportConfig.Extends[topic] = gochan.New()
		}
		p.clientProtocol = transportConfig.Extends[topic]
	}

	// kafka or gochan protocol
	client, err := cloudevents.NewClient(p.clientProtocol, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
	if err != nil {
		return err
	}
	p.client = client

	// TODO: init the inventory api
	if transportConfig.TransportType == string(transport.Multiple) && transportConfig.InventoryCredentail != nil {
		p.log.Info("Init the REST API Client")
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

func getSaramaSenderProtocol(kafkaConfig *transport.KafkaConfig, defaultTopic string) (interface{}, error) {
	saramaConfig, err := config.GetSaramaConfig(kafkaConfig)
	if err != nil {
		return nil, err
	}
	// set max message bytes to 1 MB: 1000 000 > config.ProducerConfig.MessageSizeLimitKB * 1000
	saramaConfig.Producer.MaxMessageBytes = MaxMessageKBLimit * 1000
	saramaConfig.Producer.Return.Successes = true
	sender, err := kafka_sarama.NewSender([]string{kafkaConfig.BootstrapServer},
		saramaConfig, defaultTopic)
	if err != nil {
		return nil, err
	}
	return sender, nil
}

func getConfluentSenderProtocol(kafkaCredentail *transport.KafkaConnCredential,
	defaultTopic string,
) (*kafka_confluent.Protocol, error) {
	configMap, err := config.GetConfluentConfigMapByKafkaCredential(kafkaCredentail, "")
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
				if ev.Code() == kafka.ErrAllBrokersDown {
					// ALL_BROKERS_DOWN doesn't really mean anything to librdkafka, it is just a friendly indication
					// to the application that currently there are no brokers to communicate with.
					// But librdkafka will continue to try to reconnect indefinately,
					// and it will attempt to re-send messages until message.timeout.ms or message.max.retries are exceeded.
					log.V(4).Info("Transport producer client error, ignore it for most cases", "error", ev)
				} else {
					log.Info("Thransport producer client error", "error", ev)
				}
			}
		}
	}()
}
