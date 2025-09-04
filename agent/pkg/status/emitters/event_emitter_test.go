package emitters

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// MockEventProducer implements transport.Producer for testing
type MockEventProducer struct {
	events []cloudevents.Event
}

func (m *MockEventProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *MockEventProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

func TestEventEmitter_BatchMode(t *testing.T) {
	// Initialize agent config for testing
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		// targetFilter: accept all events
		func(obj client.Object) bool { return true },
		// transformer: convert to test event
		func(obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: "test-event",
				},
				PolicyID: "test-policy",
			}
		},
		constants.EventSendModeBatch,
	)

	// Test update
	testEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-event",
		},
	}

	err := emitter.Update(testEvent)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Test send
	err = emitter.Send()
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify batch mode sent 1 event (the bundle)
	if len(producer.events) != 1 {
		t.Fatalf("Expected 1 event in batch mode, got %d", len(producer.events))
	}
}

func TestEventEmitter_SingleMode(t *testing.T) {
	// Initialize agent config for testing
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		// targetFilter: accept all events
		func(obj client.Object) bool { return true },
		// transformer: convert to test event
		func(obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: "test-event",
				},
				PolicyID: "test-policy",
			}
		},
		constants.EventSendModeSingle,
	)

	// Add multiple events
	testEvent1 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "test-event-1"},
	}
	testEvent2 := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "test-event-2"},
	}

	err := emitter.Update(testEvent1)
	if err != nil {
		t.Fatalf("Update 1 failed: %v", err)
	}

	err = emitter.Update(testEvent2)
	if err != nil {
		t.Fatalf("Update 2 failed: %v", err)
	}

	// Test send
	err = emitter.Send()
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify single mode sent 2 individual events
	if len(producer.events) != 2 {
		t.Fatalf("Expected 2 events in single mode, got %d", len(producer.events))
	}
}
