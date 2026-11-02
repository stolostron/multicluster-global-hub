// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/IBM/sarama"
	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	kafka_sarama "github.com/cloudevents/sdk-go/protocol/kafka_sarama/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/binding/format"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/cloudevents/sdk-go/v2/protocol"
	"github.com/cloudevents/sdk-go/v2/types"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var errSaramaSenderClosed = errors.New("sarama sender is closed")

// saramaSender returns from Send when Sarama accepts the message, not when Kafka
// acknowledges it. Input backpressure can block Send, but cancellation and Close
// interrupt that wait. Delivery reports finish the binding message and notify
// onDelivery, including failures that arrive after Send has returned.
type saramaSender struct {
	producer   sarama.AsyncProducer
	topic      string
	onDelivery func(*sarama.ProducerMessage, error)

	mu       sync.Mutex
	closing  bool
	sends    sync.WaitGroup
	stopping chan struct{}
	done     chan struct{}
}

// saramaDeliveryState stays local to the producer; it is not serialized to Kafka.
// Each chunk has its own state, even when several chunks share a correlation ID.
type saramaDeliveryState struct {
	message       binding.Message
	correlationID string
	once          sync.Once
}

var _ protocol.SendCloser = (*saramaSender)(nil)

// On success, newSaramaSender takes exclusive ownership of producer, including
// shutdown and both result channels. Constructor errors leave ownership with the
// caller. The caller must enable Producer.Return.Successes and
// Producer.Return.Errors. onDelivery and message Finish hooks must return promptly
// and must not wait for Close, since they run on the delivery-report goroutine.
// Sarama discards library logs by default; configure sarama.Logger during
// application setup if needed, not here, since it is process-wide.
func newSaramaSender(producer sarama.AsyncProducer, topic string,
	onDelivery func(*sarama.ProducerMessage, error),
) (*saramaSender, error) {
	if producer == nil || producer.Input() == nil || producer.Successes() == nil || producer.Errors() == nil {
		return nil, errors.New("sarama sender requires a producer with input and delivery-report channels")
	}
	if onDelivery == nil {
		return nil, errors.New("sarama sender requires a delivery-report handler")
	}
	s := &saramaSender{
		producer: producer, topic: topic, onDelivery: onDelivery,
		stopping: make(chan struct{}), done: make(chan struct{}),
	}
	go s.handleDeliveries()
	return s, nil
}

func (s *saramaSender) Send(ctx context.Context, in binding.Message, transformers ...binding.Transformer) (err error) {
	if in == nil {
		return errors.New("sarama sender requires a message")
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = in.Finish(err)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}

	// Serialize admission with Close, so Wait cannot race with a later Add.
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		return errSaramaSenderClosed
	}
	s.sends.Add(1)
	s.mu.Unlock()
	defer s.sends.Done()

	message := &sarama.ProducerMessage{Topic: s.topic}
	if topic := cecontext.TopicFrom(ctx); topic != "" {
		message.Topic = topic
	}
	if message.Topic == "" {
		return errors.New("sarama sender requires a topic")
	}
	// Preserve the existing Global Hub context key contract. Disable Sarama's
	// partitionkey extension mapping: the Confluent writer does not apply it.
	if key := kafka_confluent.MessageKeyFrom(ctx); key != "" {
		message.Key = sarama.StringEncoder(key)
	}
	if err := kafka_sarama.WriteProducerMessage(kafka_sarama.WithSkipKeyMapping(ctx), in, message, transformers...); err != nil {
		return fmt.Errorf("create sarama producer message: %w", err)
	}
	correlationID, err := saramaMessageCorrelation(message)
	if err != nil {
		return fmt.Errorf("read sarama delivery correlation: %w", err)
	}
	message.Metadata = &saramaDeliveryState{message: in, correlationID: correlationID}

	// Recheck after encoding, which may have taken time or run caller transformers.
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.stopping:
		return errSaramaSenderClosed
	case s.producer.Input() <- message:
		accepted = true
		return nil
	}
}

func (s *saramaSender) handleDeliveries() {
	defer close(s.done)
	successes, failures := s.producer.Successes(), s.producer.Errors()
	for successes != nil || failures != nil {
		select {
		case message, ok := <-successes:
			if !ok {
				successes = nil
				continue
			}
			s.reportDelivery(message, nil)
		case failure, ok := <-failures:
			if !ok {
				failures = nil
				continue
			}
			if failure != nil {
				s.reportDelivery(failure.Msg, failure.Err)
			}
		}
	}
}

func (s *saramaSender) reportDelivery(message *sarama.ProducerMessage, err error) {
	if message == nil {
		return
	}
	state, ok := message.Metadata.(*saramaDeliveryState)
	if !ok || state == nil {
		s.onDelivery(message, err)
		return
	}
	state.once.Do(func() {
		original := state.message
		state.message = nil
		_ = original.Finish(err)
		s.onDelivery(message, err)
	})
}

// Close stops new sends, interrupts blocked enqueues and drains both result
// channels through AsyncClose. A context timeout stops only this caller's wait;
// background shutdown continues. Delivery errors go to onDelivery, not Close.
func (s *saramaSender) Close(ctx context.Context) error {
	s.mu.Lock()
	if !s.closing {
		s.closing = true
		close(s.stopping)
		go func() {
			s.sends.Wait()
			s.producer.AsyncClose()
		}()
	}
	s.mu.Unlock()

	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func saramaDeliveryCorrelationFromMessage(message *sarama.ProducerMessage) string {
	if message != nil {
		if state, ok := message.Metadata.(*saramaDeliveryState); ok && state != nil {
			return state.correlationID
		}
	}
	return ""
}

// Capture correlation from the serialized message, after caller transformers.
// Structured CloudEvents keep extensions in the body instead of Kafka headers.
func saramaMessageCorrelation(message *sarama.ProducerMessage) (string, error) {
	var contentType, correlationID string
	var found, binary bool
	for _, header := range message.Headers {
		if strings.EqualFold(string(header.Key), "ce_specversion") {
			binary = true
		}
		if strings.EqualFold(string(header.Key), "ce_"+transport.DeliveryCorrelationKey) {
			correlationID, found = string(header.Value), true
		}
		if strings.EqualFold(string(header.Key), "content-type") {
			contentType = string(header.Value)
		}
	}
	// Binary payloads are application data, even when their content type names
	// a structured CloudEvents format. Only the outer headers supply correlation.
	if found || binary {
		return correlationID, nil
	}
	f := format.Lookup(contentType)
	if f == nil || message.Value == nil {
		return "", nil
	}
	payload, err := message.Value.Encode()
	if err != nil {
		return "", err
	}
	event := cloudevents.NewEvent()
	if err := f.Unmarshal(payload, &event); err != nil {
		return "", err
	}
	value, ok := event.Extensions()[transport.DeliveryCorrelationKey]
	if !ok {
		return "", nil
	}
	return types.Format(value)
}
