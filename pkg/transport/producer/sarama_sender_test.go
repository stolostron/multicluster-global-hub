// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/IBM/sarama"
	kafka_confluent "github.com/cloudevents/sdk-go/protocol/kafka_confluent/v2"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/binding"
	"github.com/cloudevents/sdk-go/v2/binding/format"
	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// Embed the pinned interface for unused transaction methods, rather than copying
// an outdated mock interface. Only these channel/lifecycle methods may be called.
type saramaSenderTestProducer struct {
	sarama.AsyncProducer
	input     chan *sarama.ProducerMessage
	successes chan *sarama.ProducerMessage
	failures  chan *sarama.ProducerError

	autoFinish    bool
	closeCalled   chan struct{}
	closeCount    atomic.Int32
	inputOnce     sync.Once
	successesOnce sync.Once
	failuresOnce  sync.Once
}

func newSaramaSenderTestProducer(buffer int, autoFinish bool) *saramaSenderTestProducer {
	return &saramaSenderTestProducer{
		input: make(chan *sarama.ProducerMessage, buffer), successes: make(chan *sarama.ProducerMessage),
		failures: make(chan *sarama.ProducerError), autoFinish: autoFinish, closeCalled: make(chan struct{}),
	}
}

func (p *saramaSenderTestProducer) Input() chan<- *sarama.ProducerMessage     { return p.input }
func (p *saramaSenderTestProducer) Successes() <-chan *sarama.ProducerMessage { return p.successes }
func (p *saramaSenderTestProducer) Errors() <-chan *sarama.ProducerError      { return p.failures }
func (p *saramaSenderTestProducer) Close() error {
	panic("sender must use AsyncClose and drain results itself")
}

func (p *saramaSenderTestProducer) AsyncClose() {
	if p.closeCount.Add(1) != 1 {
		panic("producer closed more than once")
	}
	close(p.closeCalled)
	if p.autoFinish {
		go func() {
			p.closeInput()
			for message := range p.input {
				p.successes <- message
			}
			p.finish()
		}()
	}
}

func (p *saramaSenderTestProducer) closeInput() { p.inputOnce.Do(func() { close(p.input) }) }
func (p *saramaSenderTestProducer) closeSuccesses() {
	p.successesOnce.Do(func() { close(p.successes) })
}

func (p *saramaSenderTestProducer) closeFailures() { p.failuresOnce.Do(func() { close(p.failures) }) }

func (p *saramaSenderTestProducer) finish() {
	p.closeInput()
	p.closeSuccesses()
	p.closeFailures()
}

type saramaSenderTestDelivery struct {
	message *sarama.ProducerMessage
	err     error
}

func newSaramaSenderTest(t *testing.T, buffer int, autoFinish bool,
	handler func(*sarama.ProducerMessage, error),
) (*saramaSender, *saramaSenderTestProducer, <-chan saramaSenderTestDelivery) {
	t.Helper()
	producer := newSaramaSenderTestProducer(buffer, autoFinish)
	deliveries := make(chan saramaSenderTestDelivery, 128)
	sender, err := newSaramaSender(producer, "default-topic", func(message *sarama.ProducerMessage, err error) {
		if handler != nil {
			handler(message, err)
		}
		deliveries <- saramaSenderTestDelivery{message: message, err: err}
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- sender.Close(ctx) }()
		select {
		case <-producer.closeCalled:
			if !producer.autoFinish {
				producer.finish()
			}
		case <-ctx.Done():
			t.Error("sender did not start producer shutdown")
			return
		}
		require.NoError(t, <-done)
	})
	return sender, producer, deliveries
}

func saramaSenderTestEvent() cloudevents.Event {
	event := cloudevents.NewEvent()
	event.SetID("event-id")
	event.SetSource("urn:global-hub:test")
	event.SetType("test.status")
	event.SetExtension(transport.DeliveryCorrelationKey, "correlation-id")
	_ = event.SetData(cloudevents.ApplicationJSON, []byte(`{"status":"ready"}`))
	return event
}

func saramaSenderTestReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sender test notification")
		var zero T
		return zero
	}
}

func saramaSenderTestNoNotification[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case value := <-ch:
		t.Fatalf("unexpected sender test notification: %v", value)
	default:
	}
}

func TestSaramaSenderEnqueuesBeforeDeliveryAndPreservesWireFormat(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(map[bool]string{false: "default topic", true: "context topic and key"}[override], func(t *testing.T) {
			sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
			ctx := context.Background()
			wantTopic, wantKey := "default-topic", ""
			if override {
				ctx = cecontext.WithTopic(ctx, "migration-topic")
				ctx = kafka_confluent.WithMessageKey(ctx, "migration-key")
				wantTopic, wantKey = "migration-topic", "migration-key"
			}
			event := saramaSenderTestEvent()
			event.SetExtension("partitionkey", "must-not-override-context-key")
			event.SetExtension(transport.ChunkSizeKey, 42)
			event.SetExtension(transport.ChunkOffsetKey, 21)
			finished := make(chan error, 2)
			message := binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err })
			require.NoError(t, sender.Send(ctx, message))
			saramaSenderTestNoNotification(t, finished)
			saramaSenderTestNoNotification(t, deliveries)
			queued := saramaSenderTestReceive(t, producer.input)
			require.Equal(t, wantTopic, queued.Topic)
			require.Equal(t, "correlation-id", saramaDeliveryCorrelationFromMessage(queued))
			if wantKey == "" {
				require.Nil(t, queued.Key, "partitionkey must not introduce different key mapping")
			} else {
				key, err := queued.Key.Encode()
				require.NoError(t, err)
				require.Equal(t, wantKey, string(key))
			}

			// Compare serialization with the unchanged Confluent CloudEvents writer.
			native := &kafka.Message{}
			require.NoError(t, kafka_confluent.WriteProducerMessage(ctx, binding.ToMessage(&event), native))
			payload, err := queued.Value.Encode()
			require.NoError(t, err)
			require.Equal(t, native.Value, payload)
			nativeHeaders := make(map[string]string)
			for _, header := range native.Headers {
				nativeHeaders[header.Key] = string(header.Value)
			}
			headers := make(map[string]string)
			for _, header := range queued.Headers {
				headers[string(header.Key)] = string(header.Value)
			}
			require.Equal(t, nativeHeaders, headers)

			// The existing Confluent receiver can decode this Sarama-produced record.
			received := &kafka.Message{
				TopicPartition: kafka.TopicPartition{Topic: &queued.Topic, Partition: 1, Offset: 9},
				Value:          payload,
			}
			for _, header := range queued.Headers {
				received.Headers = append(received.Headers, kafka.Header{Key: string(header.Key), Value: header.Value})
			}
			if queued.Key != nil {
				received.Key, err = queued.Key.Encode()
				require.NoError(t, err)
			}
			decoded, err := binding.ToEvent(ctx, kafka_confluent.NewMessage(received))
			require.NoError(t, err)
			require.Equal(t, event.ID(), decoded.ID())
			require.Equal(t, event.Type(), decoded.Type())
			require.Equal(t, event.Source(), decoded.Source())
			require.Equal(t, event.Data(), decoded.Data())
			require.Equal(t, "42", decoded.Extensions()[transport.ChunkSizeKey])
			require.Equal(t, wantTopic, decoded.Extensions()[kafka_confluent.KafkaTopicKey])
			require.Equal(t, "1", decoded.Extensions()[kafka_confluent.KafkaPartitionKey])
			require.Equal(t, "9", decoded.Extensions()[kafka_confluent.KafkaOffsetKey])

			producer.successes <- queued
			require.NoError(t, saramaSenderTestReceive(t, finished))
			delivery := saramaSenderTestReceive(t, deliveries)
			require.Same(t, queued, delivery.message)
			require.NoError(t, delivery.err)
		})
	}
}

func TestSaramaSenderCloudEventsClientAndPayloadOwnership(t *testing.T) {
	sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
	client, err := cloudevents.NewClient(sender)
	require.NoError(t, err)
	event := saramaSenderTestEvent()
	want := append([]byte(nil), event.Data()...)
	require.NoError(t, client.Send(context.Background(), event))
	queued := saramaSenderTestReceive(t, producer.input)
	event.Data()[0] = 'x'
	payload, err := queued.Value.Encode()
	require.NoError(t, err)
	require.Equal(t, want, payload, "queued payload must not alias the caller's mutable event data")
	producer.successes <- queued
	require.NoError(t, saramaSenderTestReceive(t, deliveries).err)
}

func TestSaramaSenderStructuredEncodingAndTransformedCorrelation(t *testing.T) {
	for _, encoding := range []string{"binary transformed", "structured", "structured input"} {
		t.Run(encoding, func(t *testing.T) {
			sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
			event := saramaSenderTestEvent()
			ctx := context.Background()
			message := binding.ToMessage(&event)
			var transformers []binding.Transformer
			wantCorrelation := "correlation-id"
			var transforms atomic.Int32
			if encoding == "binary transformed" {
				wantCorrelation = "new-correlation"
				transformers = []binding.Transformer{binding.TransformerFunc(func(_ binding.MessageMetadataReader, writer binding.MessageMetadataWriter) error {
					transforms.Add(1)
					return writer.SetExtension(transport.DeliveryCorrelationKey, wantCorrelation)
				})}
			} else {
				ctx = binding.WithForceStructured(ctx)
				if encoding == "structured input" {
					payload, err := format.JSON.Marshal(&event)
					require.NoError(t, err)
					topic := "source-topic"
					message = kafka_confluent.NewMessage(&kafka.Message{
						TopicPartition: kafka.TopicPartition{Topic: &topic},
						Value:          payload,
						Headers:        []kafka.Header{{Key: "content-type", Value: []byte(format.JSON.MediaType())}},
					})
					ctx = context.Background() // Preserve structured input without a forcing decorator.
				}
			}
			require.NoError(t, sender.Send(ctx, message, transformers...))
			queued := saramaSenderTestReceive(t, producer.input)
			require.Equal(t, wantCorrelation, saramaDeliveryCorrelationFromMessage(queued))
			if encoding == "binary transformed" {
				require.Equal(t, int32(1), transforms.Load())
			} else {
				require.Equal(t, []sarama.RecordHeader{{Key: []byte("content-type"), Value: []byte(format.JSON.MediaType())}}, queued.Headers)
			}
			producer.successes <- queued
			require.NoError(t, saramaSenderTestReceive(t, deliveries).err)
		})
	}
}

func TestSaramaSenderBinaryPayloadDoesNotSupplyDeliveryCorrelation(t *testing.T) {
	nested := saramaSenderTestEvent()
	nested.SetExtension(transport.DeliveryCorrelationKey, "unrelated-inflight-id")
	payload, err := format.JSON.Marshal(&nested)
	require.NoError(t, err)

	for _, test := range []struct {
		name             string
		payload          []byte
		outerCorrelation string
	}{
		{name: "nested event without outer correlation", payload: payload},
		{name: "outer correlation wins", payload: payload, outerCorrelation: "outer-id"},
		{name: "malformed nested event", payload: []byte("not a structured CloudEvent")},
	} {
		t.Run(test.name, func(t *testing.T) {
			sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
			event := saramaSenderTestEvent()
			event.SetExtension(transport.DeliveryCorrelationKey, nil)
			if test.outerCorrelation != "" {
				event.SetExtension(transport.DeliveryCorrelationKey, test.outerCorrelation)
			}
			require.NoError(t, event.SetData(format.JSON.MediaType(), test.payload))
			require.NoError(t, sender.Send(binding.WithForceBinary(context.Background()), binding.ToMessage(&event)))
			queued := saramaSenderTestReceive(t, producer.input)
			require.Contains(t, queued.Headers, sarama.RecordHeader{Key: []byte("ce_specversion"), Value: []byte("1.0")})
			require.Equal(t, test.outerCorrelation, saramaDeliveryCorrelationFromMessage(queued))
			gotPayload, err := queued.Value.Encode()
			require.NoError(t, err)
			require.Equal(t, test.payload, gotPayload)
			producer.successes <- queued
			require.NoError(t, saramaSenderTestReceive(t, deliveries).err)
		})
	}
}

func TestSaramaSenderDeliveryFailureAndDuplicateReport(t *testing.T) {
	sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
	event := saramaSenderTestEvent()
	finished := make(chan error, 4)
	require.NoError(t, sender.Send(context.Background(), binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err })))
	queued := saramaSenderTestReceive(t, producer.input)
	wantErr := errors.New("topic authorization failed")
	producer.failures <- &sarama.ProducerError{Msg: queued, Err: wantErr}
	require.ErrorIs(t, saramaSenderTestReceive(t, finished), wantErr)
	require.ErrorIs(t, saramaSenderTestReceive(t, deliveries).err, wantErr)

	producer.successes <- queued // A duplicate must not Finish or count the chunk twice.
	marker := &sarama.ProducerMessage{Topic: "report-barrier"}
	producer.successes <- marker
	require.Same(t, marker, saramaSenderTestReceive(t, deliveries).message)
	saramaSenderTestNoNotification(t, finished)
	saramaSenderTestNoNotification(t, deliveries)
}

func TestSaramaSenderChunkDeliveryCorrelation(t *testing.T) {
	for _, failChunk := range []bool{false, true} {
		t.Run(map[bool]string{false: "all chunks", true: "failed chunk"}[failChunk], func(t *testing.T) {
			trackerProducer := &GenericProducer{}
			tracker := trackerProducer.beginPendingDelivery(3, "current")
			sender, producer, deliveries := newSaramaSenderTest(t, 4, true, func(message *sarama.ProducerMessage, err error) {
				trackerProducer.recordDeliveryReport(kafkaDeliveryReportWithOpaque(saramaDeliveryCorrelationFromMessage(message), err))
			})
			var chunks []*sarama.ProducerMessage
			for _, id := range []string{"old", "current", "current", "current"} {
				event := saramaSenderTestEvent()
				event.SetExtension(transport.DeliveryCorrelationKey, id)
				event.SetExtension(transport.ChunkOffsetKey, len(chunks)+1)
				require.NoError(t, sender.Send(context.Background(), binding.ToMessage(&event)))
				chunks = append(chunks, saramaSenderTestReceive(t, producer.input))
			}
			producer.successes <- chunks[0]
			_ = saramaSenderTestReceive(t, deliveries)
			assertPendingNotDone(t, tracker)
			producer.successes <- chunks[1]
			_ = saramaSenderTestReceive(t, deliveries)
			assertPendingNotDone(t, tracker)
			if failChunk {
				wantErr := errors.New("chunk delivery failed")
				producer.failures <- &sarama.ProducerError{Msg: chunks[2], Err: wantErr}
				_ = saramaSenderTestReceive(t, deliveries)
				require.ErrorIs(t, waitPendingDone(t, tracker), wantErr)
			} else {
				producer.successes <- chunks[2]
				_ = saramaSenderTestReceive(t, deliveries)
				assertPendingNotDone(t, tracker)
			}
			producer.successes <- chunks[3]
			_ = saramaSenderTestReceive(t, deliveries)
			if !failChunk {
				require.NoError(t, waitPendingDone(t, tracker))
			}
		})
	}
}

func TestSaramaSenderFinishesRejectedMessagesWithTheirError(t *testing.T) {
	for _, cause := range []string{"cancelled", "encoding failed", "missing topic"} {
		t.Run(cause, func(t *testing.T) {
			sender, producer, _ := newSaramaSenderTest(t, 1, true, nil)
			event := saramaSenderTestEvent()
			finished := make(chan error, 1)
			message := binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err })
			ctx := context.Background()
			var transformers []binding.Transformer
			wantErr := context.Canceled
			switch cause {
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "encoding failed":
				wantErr = errors.New("serialization failure")
				transformers = []binding.Transformer{binding.TransformerFunc(func(binding.MessageMetadataReader, binding.MessageMetadataWriter) error {
					return wantErr
				})}
			case "missing topic":
				sender.topic = ""
				wantErr = nil
			}
			err := sender.Send(ctx, message, transformers...)
			require.Error(t, err)
			if wantErr != nil {
				require.ErrorIs(t, err, wantErr)
			}
			require.Equal(t, err, saramaSenderTestReceive(t, finished))
			saramaSenderTestNoNotification(t, producer.input)
		})
	}
}

func TestSaramaSenderCancelsBlockedEnqueue(t *testing.T) {
	sender, producer, _ := newSaramaSenderTest(t, 0, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	encoded := make(chan struct{})
	event := saramaSenderTestEvent()
	finished := make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- sender.Send(ctx, binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err }),
			binding.TransformerFunc(func(binding.MessageMetadataReader, binding.MessageMetadataWriter) error {
				close(encoded)
				return nil
			}))
	}()
	_ = saramaSenderTestReceive(t, encoded)
	cancel()
	require.ErrorIs(t, saramaSenderTestReceive(t, done), context.Canceled)
	require.ErrorIs(t, saramaSenderTestReceive(t, finished), context.Canceled)
	saramaSenderTestNoNotification(t, producer.input)
}

func TestSaramaSenderCancellationAfterEnqueueDoesNotCancelDelivery(t *testing.T) {
	sender, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	event := saramaSenderTestEvent()
	finished := make(chan error, 1)
	require.NoError(t, sender.Send(ctx, binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err })))
	queued := saramaSenderTestReceive(t, producer.input)
	cancel()
	saramaSenderTestNoNotification(t, finished)
	producer.successes <- queued
	require.NoError(t, saramaSenderTestReceive(t, finished))
	require.NoError(t, saramaSenderTestReceive(t, deliveries).err)
}

func TestSaramaSenderIgnoresNilReports(t *testing.T) {
	_, producer, deliveries := newSaramaSenderTest(t, 1, true, nil)
	producer.successes <- nil
	producer.failures <- nil
	producer.failures <- &sarama.ProducerError{Err: errors.New("report without a message")}
	marker := &sarama.ProducerMessage{Topic: "report-barrier"}
	producer.successes <- marker
	require.Same(t, marker, saramaSenderTestReceive(t, deliveries).message)
	saramaSenderTestNoNotification(t, deliveries)
}

func TestSaramaSenderCloseInterruptsBlockedEnqueue(t *testing.T) {
	sender, producer, _ := newSaramaSenderTest(t, 0, true, nil)
	encoded := make(chan struct{})
	event := saramaSenderTestEvent()
	done := make(chan error, 1)
	go func() {
		done <- sender.Send(context.Background(), binding.ToMessage(&event), binding.TransformerFunc(
			func(binding.MessageMetadataReader, binding.MessageMetadataWriter) error {
				close(encoded)
				return nil
			},
		))
	}()
	_ = saramaSenderTestReceive(t, encoded)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, sender.Close(ctx))
	require.ErrorIs(t, saramaSenderTestReceive(t, done), errSaramaSenderClosed)
	require.Equal(t, int32(1), producer.closeCount.Load())
	require.ErrorIs(t, sender.Send(context.Background(), binding.ToMessage(&event)), errSaramaSenderClosed)
}

func TestSaramaSenderTimedOutCloseContinuesDrainingBothChannels(t *testing.T) {
	sender, producer, deliveries := newSaramaSenderTest(t, 2, false, nil)
	event := saramaSenderTestEvent()
	finished := make(chan error, 2)
	var queued []*sarama.ProducerMessage
	for range 2 {
		require.NoError(t, sender.Send(context.Background(), binding.WithFinish(binding.ToMessage(&event), func(err error) { finished <- err })))
		queued = append(queued, saramaSenderTestReceive(t, producer.input))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, sender.Close(ctx), context.DeadlineExceeded)
	_ = saramaSenderTestReceive(t, producer.closeCalled)
	producer.successes <- queued[0]
	require.NoError(t, saramaSenderTestReceive(t, deliveries).err)
	require.NoError(t, saramaSenderTestReceive(t, finished))
	producer.closeSuccesses()
	wantErr := errors.New("delivery failed while closing")
	producer.failures <- &sarama.ProducerError{Msg: queued[1], Err: wantErr}
	require.ErrorIs(t, saramaSenderTestReceive(t, deliveries).err, wantErr)
	require.ErrorIs(t, saramaSenderTestReceive(t, finished), wantErr)
	producer.finish()
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	require.NoError(t, sender.Close(ctx2))
	require.Equal(t, int32(1), producer.closeCount.Load())
}

func TestSaramaSenderConcurrentSendAndClose(t *testing.T) {
	var finished, delivered atomic.Int32
	sender, producer, _ := newSaramaSenderTest(t, 16, true, func(*sarama.ProducerMessage, error) { delivered.Add(1) })
	var accepted atomic.Int32
	send := func() error {
		event := saramaSenderTestEvent()
		err := sender.Send(context.Background(), binding.WithFinish(binding.ToMessage(&event), func(error) { finished.Add(1) }))
		if err == nil {
			accepted.Add(1)
		}
		return err
	}
	for range 4 {
		require.NoError(t, send())
	}
	const count = 32
	start := make(chan struct{})
	results := make(chan error, count*2)
	for range count {
		go func() {
			<-start
			err := send()
			if errors.Is(err, errSaramaSenderClosed) {
				err = nil
			}
			results <- err
		}()
		go func() {
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			results <- sender.Close(ctx)
		}()
	}
	close(start)
	for range count * 2 {
		require.NoError(t, saramaSenderTestReceive(t, results))
	}
	require.Equal(t, int32(count+4), finished.Load())
	require.Equal(t, accepted.Load(), delivered.Load())
	require.Equal(t, int32(1), producer.closeCount.Load())
}

func TestSaramaSenderRequiresProducerAndDeliveryHandler(t *testing.T) {
	_, err := newSaramaSender(nil, "topic", func(*sarama.ProducerMessage, error) {})
	require.Error(t, err)
	for _, missing := range []string{"input", "successes", "errors", "handler"} {
		producer := newSaramaSenderTestProducer(1, true)
		handler := func(*sarama.ProducerMessage, error) {}
		switch missing {
		case "input":
			producer.input = nil
		case "successes":
			producer.successes = nil
		case "errors":
			producer.failures = nil
		case "handler":
			handler = nil
		}
		_, err := newSaramaSender(producer, "topic", handler)
		require.Error(t, err)
		require.Zero(t, producer.closeCount.Load(), "constructor failure must leave producer ownership with caller")
	}
	sender, _, _ := newSaramaSenderTest(t, 1, true, nil)
	require.Error(t, sender.Send(context.Background(), nil))
	require.Empty(t, saramaDeliveryCorrelationFromMessage(nil))
	require.Empty(t, saramaDeliveryCorrelationFromMessage(&sarama.ProducerMessage{}))
}

func TestSaramaSenderWithRealAsyncProducerAndMockBroker(t *testing.T) {
	for _, result := range []sarama.KError{sarama.ErrNoError, sarama.ErrTopicAuthorizationFailed} {
		t.Run(result.Error(), func(t *testing.T) {
			broker := sarama.NewMockBroker(t, 1)
			defer broker.Close()
			const topic = "sender-unit-topic"
			broker.SetHandlerByMap(map[string]sarama.MockResponse{
				"ApiVersionsRequest": sarama.NewMockApiVersionsResponse(t).SetApiKeys([]sarama.ApiVersionsResponseKey{
					{ApiKey: 0, MinVersion: 0, MaxVersion: 7},
					{ApiKey: 3, MinVersion: 0, MaxVersion: 7},
					{ApiKey: 18, MinVersion: 0, MaxVersion: 3},
				}),
				"MetadataRequest": sarama.NewMockMetadataResponse(t).SetBroker(broker.Addr(), 1).SetLeader(topic, 0, 1),
				"ProduceRequest":  sarama.NewMockProduceResponse(t).SetError(topic, 0, result),
			})
			cfg := sarama.NewConfig()
			cfg.Version = sarama.V2_0_0_0
			cfg.Producer.Return.Successes = true
			cfg.Producer.Return.Errors = true
			cfg.Producer.Retry.Max = 0
			cfg.Producer.Timeout = time.Second
			cfg.Net.DialTimeout = time.Second
			cfg.Net.ReadTimeout = time.Second
			cfg.Net.WriteTimeout = time.Second
			producer, err := sarama.NewAsyncProducer([]string{broker.Addr()}, cfg)
			require.NoError(t, err)
			deliveries := make(chan saramaSenderTestDelivery, 1)
			sender, err := newSaramaSender(producer, topic, func(message *sarama.ProducerMessage, err error) {
				deliveries <- saramaSenderTestDelivery{message: message, err: err}
			})
			require.NoError(t, err)
			defer func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, sender.Close(ctx))
			}()
			event := saramaSenderTestEvent()
			require.NoError(t, sender.Send(context.Background(), binding.ToMessage(&event)))
			delivery := saramaSenderTestReceive(t, deliveries)
			if result == sarama.ErrNoError {
				require.NoError(t, delivery.err)
			} else {
				require.ErrorIs(t, delivery.err, result)
			}
			require.Equal(t, topic, delivery.message.Topic)
			require.Equal(t, int32(0), delivery.message.Partition)
			require.Equal(t, "correlation-id", saramaDeliveryCorrelationFromMessage(delivery.message))
		})
	}
}

func TestSaramaSenderLateReportsDoNotConfirmNextPublish(t *testing.T) {
	trackerProducer := &GenericProducer{}
	sender, producer, deliveries := newSaramaSenderTest(t, 4, true, func(message *sarama.ProducerMessage, err error) {
		trackerProducer.recordDeliveryReport(kafkaDeliveryReportWithOpaque(saramaDeliveryCorrelationFromMessage(message), err))
	})
	first := trackerProducer.beginPendingDelivery(2, "first")
	require.NotNil(t, first)
	var queued []*sarama.ProducerMessage
	for _, id := range []string{"first", "first", "second", "second"} {
		event := saramaSenderTestEvent()
		event.SetExtension(transport.DeliveryCorrelationKey, id)
		require.NoError(t, sender.Send(context.Background(), binding.ToMessage(&event)))
		queued = append(queued, saramaSenderTestReceive(t, producer.input))
	}
	trackerProducer.clearPendingDelivery(first) // Simulate a timed-out confirmation.
	second := trackerProducer.beginPendingDelivery(2, "second")
	require.NotNil(t, second)
	producer.successes <- queued[0]
	_ = saramaSenderTestReceive(t, deliveries)
	producer.failures <- &sarama.ProducerError{Msg: queued[1], Err: errors.New("late first-publish failure")}
	_ = saramaSenderTestReceive(t, deliveries)
	assertPendingNotDone(t, second)
	producer.successes <- queued[2]
	_ = saramaSenderTestReceive(t, deliveries)
	assertPendingNotDone(t, second)
	producer.successes <- queued[3]
	_ = saramaSenderTestReceive(t, deliveries)
	require.NoError(t, waitPendingDone(t, second))
}
