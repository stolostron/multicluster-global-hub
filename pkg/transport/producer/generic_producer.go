// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"fmt"
	"sync"
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
	"github.com/stolostron/multicluster-global-hub/pkg/transport/utils"
)

type GenericProducer struct {
	log                  *zap.SugaredLogger
	ceProtocol           interface{}
	ceClient             cloudevents.Client
	kafkaProducer        *kafka.Producer
	messageSizeLimit     int
	eventErrorHandler    func(event *kafka.Message)
	sendMu               sync.Mutex
	pendingDeliveryMu    sync.Mutex
	pendingDelivery      *pendingDeliveryTracker
	flushConfirmMu       sync.Mutex
	flushConfirming      bool
	flushFirstErr        error
	asyncDeliveryMu      sync.Mutex
	asyncDeliveryPending int
}

type pendingDeliveryTracker struct {
	correlationID string
	remaining     int
	firstErr      error
	done          chan error
}

func NewGenericProducer(transportConfig *transport.TransportInternalConfig, topic string,
	eventErrorHandler func(event *kafka.Message),
) (*GenericProducer, error) {
	genericProducer := &GenericProducer{
		log:               logger.ZapLogger(fmt.Sprintf("%s-producer", transportConfig.TransportType)),
		messageSizeLimit:  config.MaxSizeToChunk,
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
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	return p.sendEventLocked(ctx, evt)
}

func (p *GenericProducer) sendEventLocked(ctx context.Context, evt cloudevents.Event) error {
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

// SendEventWithDeliveryConfirmation sends an event and waits for all Kafka delivery
// reports for that event (including every chunk). Non-Kafka transports fall back to SendEvent.
func (p *GenericProducer) SendEventWithDeliveryConfirmation(
	ctx context.Context, evt cloudevents.Event, timeout time.Duration,
) error {
	if p.kafkaProducer == nil {
		p.sendMu.Lock()
		defer p.sendMu.Unlock()
		return p.sendEventLocked(ctx, evt)
	}

	p.sendMu.Lock()
	defer p.sendMu.Unlock()

	// Drain in-flight messages from ordinary SendEvent traffic before capturing
	// delivery errors, so unrelated failures are not attributed to this publish.
	p.drainKafkaProducerQueue()
	p.waitAsyncDeliveryReportsSettled(500 * time.Millisecond)

	if err := p.sendEventLocked(ctx, evt); err != nil {
		return err
	}

	p.beginFlushConfirmation()
	defer p.endFlushConfirmation()

	deadline := time.Now().Add(timeout)
	for {
		if err := p.flushDeliveryError(); err != nil {
			return err
		}

		remaining := p.kafkaProducer.Flush(250)
		if err := p.flushDeliveryError(); err != nil {
			return err
		}
		if remaining == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf(
				"timed out waiting for %d kafka delivery reports after %v",
				remaining, timeout,
			)
		}
	}
}

func (p *GenericProducer) beginFlushConfirmation() {
	p.flushConfirmMu.Lock()
	defer p.flushConfirmMu.Unlock()
	p.flushConfirming = true
	p.flushFirstErr = nil
}

func (p *GenericProducer) endFlushConfirmation() {
	p.flushConfirmMu.Lock()
	defer p.flushConfirmMu.Unlock()
	p.flushConfirming = false
	p.flushFirstErr = nil
}

func (p *GenericProducer) recordFlushDeliveryError(err error) {
	if err == nil {
		return
	}
	p.flushConfirmMu.Lock()
	defer p.flushConfirmMu.Unlock()
	if p.flushConfirming && p.flushFirstErr == nil {
		p.flushFirstErr = err
	}
}

func (p *GenericProducer) flushDeliveryError() error {
	p.flushConfirmMu.Lock()
	defer p.flushConfirmMu.Unlock()
	return p.flushFirstErr
}

func (p *GenericProducer) drainKafkaProducerQueue() {
	for {
		if p.kafkaProducer.Flush(250) == 0 {
			return
		}
	}
}

func (p *GenericProducer) beginAsyncDeliveryReport() {
	p.asyncDeliveryMu.Lock()
	p.asyncDeliveryPending++
	p.asyncDeliveryMu.Unlock()
}

func (p *GenericProducer) endAsyncDeliveryReport() {
	p.asyncDeliveryMu.Lock()
	p.asyncDeliveryPending--
	p.asyncDeliveryMu.Unlock()
}

func (p *GenericProducer) waitAsyncDeliveryReportsSettled(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p.asyncDeliveryMu.Lock()
		pending := p.asyncDeliveryPending
		p.asyncDeliveryMu.Unlock()
		if pending == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (p *GenericProducer) expectedDeliveryReports(evt cloudevents.Event) int {
	expected := len(p.splitPayloadIntoChunks(evt.Data()))
	if expected == 0 {
		return 1
	}
	return expected
}

func (p *GenericProducer) beginPendingDelivery(expected int, correlationID string) *pendingDeliveryTracker {
	p.pendingDeliveryMu.Lock()
	defer p.pendingDeliveryMu.Unlock()
	if p.pendingDelivery != nil {
		return nil
	}
	tracker := &pendingDeliveryTracker{
		correlationID: correlationID,
		remaining:     expected,
		done:          make(chan error, 1),
	}
	p.pendingDelivery = tracker
	return tracker
}

func (p *GenericProducer) clearPendingDelivery(tracker *pendingDeliveryTracker) {
	p.pendingDeliveryMu.Lock()
	defer p.pendingDeliveryMu.Unlock()
	if p.pendingDelivery == tracker {
		p.pendingDelivery = nil
	}
}

func (p *GenericProducer) recordDeliveryReport(m *kafka.Message) {
	p.beginAsyncDeliveryReport()
	defer p.endAsyncDeliveryReport()

	if m != nil && m.TopicPartition.Error != nil {
		p.recordFlushDeliveryError(m.TopicPartition.Error)
	}

	correlationID := deliveryCorrelationFromMessage(m)
	if correlationID == "" {
		return
	}

	var deliveryErr error
	if m.TopicPartition.Error != nil {
		deliveryErr = m.TopicPartition.Error
	}

	p.pendingDeliveryMu.Lock()
	tracker := p.pendingDelivery
	if tracker == nil || correlationID != tracker.correlationID {
		p.pendingDeliveryMu.Unlock()
		return
	}

	if deliveryErr != nil && tracker.firstErr == nil {
		tracker.firstErr = deliveryErr
		p.pendingDelivery = nil
		p.pendingDeliveryMu.Unlock()
		tracker.done <- deliveryErr
		return
	}

	tracker.remaining--
	if tracker.remaining <= 0 {
		p.pendingDelivery = nil
		pendingErr := tracker.firstErr
		p.pendingDeliveryMu.Unlock()
		tracker.done <- pendingErr
		return
	}
	p.pendingDeliveryMu.Unlock()
}

func deliveryCorrelationFromMessage(m *kafka.Message) string {
	if m == nil {
		return ""
	}
	if correlationID, ok := m.Opaque.(string); ok && correlationID != "" {
		return correlationID
	}
	headerKey := "ce_" + transport.DeliveryCorrelationKey
	for _, header := range m.Headers {
		if header.Key == headerKey {
			return string(header.Value)
		}
	}
	return ""
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
		producer, kafkaProtocol, err := getConfluentSenderProtocol(p.log, transportConfig.KafkaCredential, topic)
		if err != nil {
			return err
		}

		eventChan, err := kafkaProtocol.Events()
		if err != nil {
			return err
		}
		handleProducerEvents(p, eventChan, transportConfig.FailureThreshold)
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
	chunks := make([][]byte, 0, len(payload)/p.messageSizeLimit+1)
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

func getConfluentSenderProtocol(logger *zap.SugaredLogger, kafkaCredentail *transport.KafkaConfig,
	defaultTopic string,
) (*kafka.Producer, *kafka_confluent.Protocol, error) {
	configMap, err := config.GetConfluentConfigMapByKafkaCredential(kafkaCredentail, "", 0)
	if err != nil {
		return nil, nil, err
	}
	logger.Debugw("the configurations applied to the Kafka producer", "configMap",
		utils.FilterSensitiveKafkaConfig(configMap))

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

func handleProducerEvents(p *GenericProducer, eventChan chan kafka.Event, transportFailureThreshold int) {
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
					p.recordDeliveryReport(m)
					if p.eventErrorHandler != nil {
						p.eventErrorHandler(m)
					}
					p.log.Warnw("delivery failed", "error", m.TopicPartition.Error)
				} else {
					p.recordDeliveryReport(m)
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
					p.log.Debugw("transport producer client error(ALL_BROKERS_DOWN), ignore it for most cases", "error", ev)
				} else {
					p.log.Warnw("transport producer client error", "error", ev)

					errorCount++
					if errorCount >= transportFailureThreshold {
						p.log.Panicf("transport producer error > 10 in 5 minutes, error: %v", ev)
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
