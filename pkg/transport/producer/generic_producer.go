// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"fmt"
	"time"

	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cectx "github.com/cloudevents/sdk-go/v2/context"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/config"
)

const (
	MaxMessageKBLimit    = 1024
	DefaultMessageKBSize = 102400 // disable the chunking.
)

type GenericProducer struct {
	log               *zap.SugaredLogger
	ceProtocol        interface{}
	ceClient          cloudevents.Client
	kafkaProducer     *kafka.Producer
	messageSizeLimit  int
	eventErrorHandler func(event *kafka.Message)
}

func NewGenericProducer(transportConfig *transport.TransportInternalConfig, topic string,
	eventErrorHandler func(event *kafka.Message),
) (*GenericProducer, error) {
	genericProducer := &GenericProducer{
		log:               logger.ZapLogger(fmt.Sprintf("%s-producer", transportConfig.TransportType)),
		messageSizeLimit:  DefaultMessageKBSize * 1000,
		eventErrorHandler: eventErrorHandler,
	}
	err := genericProducer.initClient(transportConfig, topic)
	if err != nil {
		return nil, err
	}

	return genericProducer, nil
}

func (p *GenericProducer) KafkaProducer() *kafka.Producer {
	return p.kafkaProducer
}

func (p *GenericProducer) Protocol() *kafka_confluent.Protocol {
	return p.ceProtocol.(*kafka_confluent.Protocol)
}

func (p *GenericProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	// cloudevent kafka/gochan client
	// message key
	evtCtx := cectx.WithLogger(ctx, logger.ZapLogger("cloudevents"))
	if kafka_confluent.MessageKeyFrom(ctx) == "" {
		evtCtx = kafka_confluent.WithMessageKey(ctx, evt.Type())
	}

	// data
	payloadBytes := evt.Data()
	chunks := p.splitPayloadIntoChunks(payloadBytes)
	if len(chunks) <= 1 {
		if ret := p.ceClient.Send(evtCtx, evt); cloudevents.IsUndelivered(ret) {
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
		if result := p.ceClient.Send(evtCtx, evt); cloudevents.IsUndelivered(result) {
			return fmt.Errorf("failed to send events to transport: %v", result)
		}
	}
	return nil
}

// Reconnect close the previous producer state and init a new producer
func (p *GenericProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	// cloudevent kafka/gochan client
	closer, ok := p.ceProtocol.(protocol.Closer)
	if ok {
		if err := closer.Close(context.Background()); err != nil {
			return fmt.Errorf("failed to close the previous producer: %w", err)
		}
	}
	return p.initClient(config, topic)
}

// initClient will init/update the client, clientProtocol and messageLimitSize based on the transportConfig
func (p *GenericProducer) initClient(transportConfig *transport.TransportInternalConfig, topic string) error {
	switch transportConfig.TransportType {
	case string(transport.Kafka):
		producer, kafkaProtocol, err := getConfluentSenderProtocol(transportConfig.KafkaCredential, topic)
		if err != nil {
			return err
		}

		eventChan, err := kafkaProtocol.Events()
		if err != nil {
			return err
		}
		handleProducerEvents(p.log, eventChan, transportConfig.FailureThreshold, p.eventErrorHandler)
		p.ceProtocol = kafkaProtocol
		p.kafkaProducer = producer
	case string(transport.Chan):
		if transportConfig.Extends == nil {
			transportConfig.Extends = make(map[string]interface{})
		}
		if _, found := transportConfig.Extends[topic]; !found {
			transportConfig.Extends[topic] = gochan.New()
		}
		p.ceProtocol = transportConfig.Extends[topic]
	default:
		return fmt.Errorf("transport-type - %s is not a valid option", transportConfig.TransportType)
	}

	// kafka or gochan protocol
	if p.ceProtocol != nil {
		client, err := cloudevents.NewClient(p.ceProtocol, cloudevents.WithTimeNow(), cloudevents.WithUUIDs())
		if err != nil {
			return err
		}
		p.ceClient = client
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

func getConfluentSenderProtocol(kafkaCredentail *transport.KafkaConfig,
	defaultTopic string,
) (*kafka.Producer, *kafka_confluent.Protocol, error) {
	configMap, err := config.GetConfluentConfigMapByKafkaCredential(kafkaCredentail, "")
	if err != nil {
		return nil, nil, err
	}
	producer, err := kafka.NewProducer(configMap)
	if err != nil {
		return nil, nil, err
	}
	protocol, err := kafka_confluent.New(kafka_confluent.WithSenderTopic(defaultTopic),
		kafka_confluent.WithSender(producer))
	if err != nil {
		return nil, nil, err
	}
	return producer, protocol, nil
}

func handleProducerEvents(log *zap.SugaredLogger, eventChan chan kafka.Event, transportFailureThreshold int,
	eventErrorHandler func(event *kafka.Message),
) {
	// Listen to all the events on the default events channel
	// It's important to read these events otherwise the events channel will eventually fill up
	go func() {
		errorCount := 0
		var lastErrorTime time.Time
		for e := range eventChan {
			switch ev := e.(type) {
			case *kafka.Message:
				// The message delivery report, indicating success or
				// permanent failure after retries have been exhausted.
				// Application level retries won't help since the client
				// is already configured to do that.
				m := ev
				if m.TopicPartition.Error != nil {
					if eventErrorHandler != nil {
						eventErrorHandler(m)
					}
					log.Warnw("delivery failed", "error", m.TopicPartition.Error)
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
					log.Debugw("transport producer client error(ALL_BROKERS_DOWN), ignore it for most cases", "error", ev)
				} else {
					log.Warnw("transport producer client error", "error", ev)

					errorCount++
					if errorCount >= transportFailureThreshold {
						log.Panicf("transport producer error > 10 in 5 minutes, error: %v", ev)
					}
					// return panic when error more than 10 times in 5 minites
					if lastErrorTime.Add(5 * time.Minute).Before(time.Now()) {
						errorCount = 0
					}
					lastErrorTime = time.Now()
				}
			}
		}
	}()
}
