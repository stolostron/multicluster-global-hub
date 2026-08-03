// Copyright (c) 2023 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package producer

import (
	"errors"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/stretchr/testify/require"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGenericProducer(t *testing.T) {
	p := &GenericProducer{}
	tranConfig := &transport.TransportInternalConfig{
		TransportType: string(transport.Rest),
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic:   "gh-spec",
			StatusTopic: "gh-status",
		},
	}
	err := p.initClient(tranConfig, tranConfig.KafkaCredential.StatusTopic)
	require.Equal(t, "transport-type - rest is not a valid option", err.Error())
}

func Test_handleProducerEvents(t *testing.T) {
	tests := []struct {
		name                      string
		event                     kafka.Event
		transportFailureThreshold int
	}{
		{
			name:                      "kafka error",
			transportFailureThreshold: 10,
			event:                     kafka.NewError(kafka.ErrFail, "errStr", false),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &GenericProducer{log: logger.DefaultZapLogger()}
			eventChan := make(chan kafka.Event)
			go handleProducerEvents(p, eventChan, tt.transportFailureThreshold)
			eventChan <- tt.event
		})
	}
}

func TestRecordDeliveryReport_multiChunkSuccess(t *testing.T) {
	p := &GenericProducer{}
	const correlationID = "corr-multi"
	tracker := p.beginPendingDelivery(3, correlationID)
	require.NotNil(t, tracker)

	p.recordDeliveryReport(kafkaDeliveryReportWithOpaque(correlationID, nil))
	p.recordDeliveryReport(kafkaDeliveryReportWithOpaque(correlationID, nil))
	assertPendingNotDone(t, tracker)

	p.recordDeliveryReport(kafkaDeliveryReportWithOpaque(correlationID, nil))
	require.NoError(t, waitPendingDone(t, tracker))
}

func TestRecordDeliveryReport_firstChunkFailure(t *testing.T) {
	p := &GenericProducer{}
	const correlationID = "corr-fail"
	tracker := p.beginPendingDelivery(3, correlationID)
	require.NotNil(t, tracker)

	sendErr := errors.New("Topic authorization failed")
	p.recordDeliveryReport(kafkaDeliveryReport(correlationID, sendErr))

	require.Equal(t, sendErr, waitPendingDone(t, tracker))
}

func TestRecordDeliveryReport_ignoredWithoutTracker(t *testing.T) {
	p := &GenericProducer{}
	require.NotPanics(t, func() {
		p.recordDeliveryReport(kafkaDeliveryReport("corr-orphan", nil))
		p.recordDeliveryReport(kafkaDeliveryReport("corr-orphan", errors.New("ignored")))
	})
}

func TestRecordDeliveryReport_ignoresUnrelatedCorrelation(t *testing.T) {
	p := &GenericProducer{}
	const correlationID = "corr-current"
	tracker := p.beginPendingDelivery(2, correlationID)
	require.NotNil(t, tracker)

	// Pre-existing or unrelated delivery reports must not advance this tracker.
	p.recordDeliveryReport(kafkaDeliveryReport("corr-old", nil))
	p.recordDeliveryReport(kafkaDeliveryReport("", nil))
	assertPendingNotDone(t, tracker)

	p.recordDeliveryReport(kafkaDeliveryReport(correlationID, nil))
	p.recordDeliveryReport(kafkaDeliveryReport(correlationID, nil))
	require.NoError(t, waitPendingDone(t, tracker))

	// Late unrelated report after completion is ignored.
	require.NotPanics(t, func() {
		p.recordDeliveryReport(kafkaDeliveryReport("corr-old", nil))
	})
}

func TestRecordDeliveryReport_lateReportAfterTrackerCleared(t *testing.T) {
	p := &GenericProducer{}
	const correlationA = "corr-a"
	const correlationB = "corr-b"

	trackerA := p.beginPendingDelivery(2, correlationA)
	require.NotNil(t, trackerA)
	// Simulate confirmation timeout clearing the tracker before all reports arrive.
	p.clearPendingDelivery(trackerA)

	trackerB := p.beginPendingDelivery(2, correlationB)
	require.NotNil(t, trackerB)

	// Late report from tracker A must not satisfy tracker B.
	p.recordDeliveryReport(kafkaDeliveryReport(correlationA, nil))
	assertPendingNotDone(t, trackerB)

	p.recordDeliveryReport(kafkaDeliveryReport(correlationB, nil))
	p.recordDeliveryReport(kafkaDeliveryReport(correlationB, nil))
	require.NoError(t, waitPendingDone(t, trackerB))
}

func TestDeliveryCorrelationFromMessage(t *testing.T) {
	require.Equal(t, "abc", deliveryCorrelationFromMessage(kafkaDeliveryReport("abc", nil)))
	require.Equal(t, "opaque-id", deliveryCorrelationFromMessage(kafkaDeliveryReportWithOpaque("opaque-id", nil)))
	require.Equal(t, "", deliveryCorrelationFromMessage(kafkaDeliveryReport("", nil)))
	require.Equal(t, "", deliveryCorrelationFromMessage(nil))
}

func TestExpectedDeliveryReports(t *testing.T) {
	p := &GenericProducer{messageSizeLimit: 4}

	evt := cloudeventsEventWithData([]byte("123456789"))
	require.Equal(t, 3, p.expectedDeliveryReports(evt))

	evt = cloudeventsEventWithData(nil)
	require.Equal(t, 1, p.expectedDeliveryReports(evt))
}

func TestBeginPendingDelivery_rejectsConcurrentTracker(t *testing.T) {
	p := &GenericProducer{}
	first := p.beginPendingDelivery(1, "corr-1")
	require.NotNil(t, first)
	require.Nil(t, p.beginPendingDelivery(1, "corr-2"))
}

func TestRecordFlushDeliveryError_ignoredWhenNotConfirming(t *testing.T) {
	p := &GenericProducer{}
	p.recordFlushDeliveryError(errors.New("unrelated"))
	require.NoError(t, p.flushDeliveryError())
}

func TestFlushConfirmation_ignoresPredrainDeliveryErrors(t *testing.T) {
	p := &GenericProducer{}

	// Simulate an ordinary SendEvent failure still being processed before confirmation.
	p.recordDeliveryReport(kafkaDeliveryReport("", errors.New("unrelated ordinary send failure")))
	require.NoError(t, p.flushDeliveryError())

	p.waitAsyncDeliveryReportsSettled(time.Second)

	p.beginFlushConfirmation()
	defer p.endFlushConfirmation()

	p.recordDeliveryReport(kafkaDeliveryReport("", nil))
	require.NoError(t, p.flushDeliveryError())
}

func TestFlushConfirmation_unrelatedErrorDuringConfirmationStillFails(t *testing.T) {
	p := &GenericProducer{}

	p.beginFlushConfirmation()
	defer p.endFlushConfirmation()

	unrelatedErr := errors.New("unrelated failure during confirmation window")
	p.recordDeliveryReport(kafkaDeliveryReport("", unrelatedErr))
	require.Equal(t, unrelatedErr, p.flushDeliveryError())
}

func TestWaitAsyncDeliveryReportsSettled_waitsForInFlightHandler(t *testing.T) {
	p := &GenericProducer{}
	p.beginAsyncDeliveryReport()

	done := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		p.endAsyncDeliveryReport()
		close(done)
	}()

	p.waitAsyncDeliveryReportsSettled(time.Second)
	<-done
}

func assertPendingNotDone(t *testing.T, tracker *pendingDeliveryTracker) {
	t.Helper()
	select {
	case err := <-tracker.done:
		t.Fatalf("expected pending delivery to remain open, got %v", err)
	default:
	}
}

func waitPendingDone(t *testing.T, tracker *pendingDeliveryTracker) error {
	t.Helper()
	select {
	case err := <-tracker.done:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for pending delivery")
		return nil
	}
}

func kafkaDeliveryReport(correlationID string, deliveryErr error) *kafka.Message {
	msg := &kafka.Message{}
	if correlationID != "" {
		msg.Headers = []kafka.Header{{
			Key:   "ce_" + transport.DeliveryCorrelationKey,
			Value: []byte(correlationID),
		}}
	}
	if deliveryErr != nil {
		msg.TopicPartition.Error = deliveryErr
	}
	return msg
}

func kafkaDeliveryReportWithOpaque(correlationID string, deliveryErr error) *kafka.Message {
	msg := &kafka.Message{Opaque: correlationID}
	if deliveryErr != nil {
		msg.TopicPartition.Error = deliveryErr
	}
	return msg
}

func cloudeventsEventWithData(data []byte) cloudevents.Event {
	evt := cloudevents.NewEvent()
	evt.SetType("test.event")
	_ = evt.SetData(cloudevents.ApplicationJSON, data)
	return evt
}
