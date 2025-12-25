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
		EventMode:   string(constants.EventSendModeBatch),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil, // runtimeClient not needed for this test
		// filter: accept all events
		func(obj client.Object) bool { return true },
		// transform: convert to test event
		func(runtimeClient client.Client, obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: "test-event",
				},
				PolicyID: "test-policy",
			}
		},
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
		EventMode:   string(constants.EventSendModeSingle),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil, // runtimeClient not needed for this test
		// filter: accept all events
		func(obj client.Object) bool { return true },
		// transform: convert to test event
		func(runtimeClient client.Client, obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: "test-event",
				},
				PolicyID: "test-policy",
			}
		},
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

func TestEventEmitter_MultipleRoundsSendAndAppend(t *testing.T) {
	// This test verifies that e.events[:0] correctly clears events
	// and subsequent appends work correctly across multiple send cycles
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
		EventMode:   string(constants.EventSendModeBatch),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	eventCounter := 0
	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil,
		func(obj client.Object) bool { return true },
		func(runtimeClient client.Client, obj client.Object) interface{} {
			eventCounter++
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: obj.GetName(),
					Message:   "event-" + obj.GetName(),
				},
				PolicyID: "policy-" + obj.GetName(),
			}
		},
	)

	// Round 1: Add 2 events and send
	for i := 0; i < 2; i++ {
		testEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "round1-event-" + string(rune('a'+i))},
		}
		if err := emitter.Update(testEvent); err != nil {
			t.Fatalf("Round 1 Update %d failed: %v", i, err)
		}
	}
	if err := emitter.Send(); err != nil {
		t.Fatalf("Round 1 Send failed: %v", err)
	}

	// Verify round 1 sent 1 bundle with 2 events
	if len(producer.events) != 1 {
		t.Fatalf("Expected 1 bundle after round 1, got %d", len(producer.events))
	}

	// Round 2: Add 3 more events and send
	for i := 0; i < 3; i++ {
		testEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "round2-event-" + string(rune('a'+i))},
		}
		if err := emitter.Update(testEvent); err != nil {
			t.Fatalf("Round 2 Update %d failed: %v", i, err)
		}
	}
	if err := emitter.Send(); err != nil {
		t.Fatalf("Round 2 Send failed: %v", err)
	}

	// Verify round 2 sent another bundle (total 2 bundles)
	if len(producer.events) != 2 {
		t.Fatalf("Expected 2 bundles after round 2, got %d", len(producer.events))
	}

	// Verify the events in each bundle are independent (not corrupted by slice reuse)
	var round1Events, round2Events []event.RootPolicyEvent
	if err := producer.events[0].DataAs(&round1Events); err != nil {
		t.Fatalf("Failed to decode round 1 events: %v", err)
	}
	if err := producer.events[1].DataAs(&round2Events); err != nil {
		t.Fatalf("Failed to decode round 2 events: %v", err)
	}

	if len(round1Events) != 2 {
		t.Fatalf("Expected 2 events in round 1 bundle, got %d", len(round1Events))
	}
	if len(round2Events) != 3 {
		t.Fatalf("Expected 3 events in round 2 bundle, got %d", len(round2Events))
	}

	// Verify round 1 events are not corrupted by round 2 data
	for _, e := range round1Events {
		if e.EventName == "" || !contains(e.EventName, "round1") {
			t.Errorf("Round 1 event corrupted: %+v", e)
		}
	}
	for _, e := range round2Events {
		if e.EventName == "" || !contains(e.EventName, "round2") {
			t.Errorf("Round 2 event corrupted: %+v", e)
		}
	}

	t.Logf("Successfully verified %d events across 2 send rounds", eventCounter)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEventEmitter_BundleSizeLimit(t *testing.T) {
	// Initialize agent config for testing
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
		EventMode:   string(constants.EventSendModeBatch),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	// Create a large payload that will exceed the size limit when accumulated
	largeMessage := make([]byte, 100*1024) // 100KB per event
	for i := range largeMessage {
		largeMessage[i] = 'x'
	}
	largeMessageStr := string(largeMessage)

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil,
		func(obj client.Object) bool { return true },
		func(runtimeClient client.Client, obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: obj.GetName(),
					Message:   largeMessageStr, // ~100KB message
				},
				PolicyID: "test-policy",
			}
		},
	)

	// Add 10 events (10 * 100KB = ~1MB, exceeds 800KB limit)
	for i := 0; i < 10; i++ {
		testEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-event-" + string(rune('0'+i)),
			},
		}
		err := emitter.Update(testEvent)
		if err != nil {
			t.Fatalf("Update %d failed: %v", i, err)
		}
	}

	// Send remaining events
	err := emitter.Send()
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Should have sent more than 1 bundle due to size limit
	if len(producer.events) < 2 {
		t.Fatalf("Expected at least 2 bundles due to size limit, got %d", len(producer.events))
	}
	t.Logf("Successfully sent %d bundles due to size limit enforcement", len(producer.events))
}

func TestEventEmitter_SingleMode_NoSizeChecking(t *testing.T) {
	// Verify that single mode does NOT enforce bundle size limits
	// Events accumulate and are sent individually regardless of total size
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
		EventMode:   string(constants.EventSendModeSingle),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	// Create large events (100KB each)
	largeMessage := make([]byte, 100*1024)
	for i := range largeMessage {
		largeMessage[i] = 'x'
	}
	largeMessageStr := string(largeMessage)

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil,
		func(obj client.Object) bool { return true },
		func(runtimeClient client.Client, obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: obj.GetName(),
					Message:   largeMessageStr,
				},
				PolicyID: "test-policy",
			}
		},
	)

	// Add 10 large events (10 * 100KB = ~1MB total)
	// In single mode, these should all accumulate without triggering auto-send
	for i := 0; i < 10; i++ {
		testEvent := &corev1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-event-" + string(rune('0'+i)),
			},
		}
		err := emitter.Update(testEvent)
		if err != nil {
			t.Fatalf("Update %d failed: %v", i, err)
		}
	}

	// Before calling Send(), no events should have been sent
	if len(producer.events) != 0 {
		t.Fatalf("Expected 0 events before Send() in single mode, got %d", len(producer.events))
	}

	// Now send - should send 10 individual CloudEvents
	err := emitter.Send()
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Should have sent exactly 10 individual events (one per event)
	if len(producer.events) != 10 {
		t.Fatalf("Expected 10 individual events in single mode, got %d", len(producer.events))
	}

	t.Logf("Successfully sent %d individual events in single mode without size checking", len(producer.events))
}

func TestEventEmitter_SingleEventExceedsLimit(t *testing.T) {
	// Test that a single event exceeding the bundle size limit is properly rejected
	testConfig := &configs.AgentConfig{
		LeafHubName: "test-hub",
		EventMode:   string(constants.EventSendModeBatch),
	}
	configs.SetAgentConfig(testConfig)

	producer := &MockEventProducer{}

	// Create an extremely large event that exceeds 800KB limit by itself
	hugeMessage := make([]byte, 900*1024) // 900KB - exceeds 800KB limit
	for i := range hugeMessage {
		hugeMessage[i] = 'x'
	}
	hugeMessageStr := string(hugeMessage)

	emitter := NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		nil,
		func(obj client.Object) bool { return true },
		func(runtimeClient client.Client, obj client.Object) interface{} {
			return event.RootPolicyEvent{
				BaseEvent: event.BaseEvent{
					EventName: obj.GetName(),
					Message:   hugeMessageStr, // 900KB message
				},
				PolicyID: "test-policy",
			}
		},
	)

	// Try to add a single huge event - should fail with specific error
	testEvent := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: "huge-event",
		},
	}

	err := emitter.Update(testEvent)
	if err == nil {
		t.Fatal("Expected error for single event exceeding bundle size limit, got nil")
	}

	if !contains(err.Error(), "single event exceeds bundle size limit") {
		t.Fatalf("Expected 'single event exceeds bundle size limit' error, got: %v", err)
	}

	t.Logf("Correctly rejected single event exceeding limit: %v", err)
}
