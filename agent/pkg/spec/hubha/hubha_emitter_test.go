// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// mockProducer implements transport.Producer for testing
type mockProducer struct {
	events []cloudevents.Event
}

func (m *mockProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.events = append(m.events, evt)
	return nil
}

func (m *mockProducer) Stop() {}

func (m *mockProducer) Reconnect(config *transport.TransportInternalConfig, clusterName string) error {
	return nil
}

func TestHubHAEmitter_EventType(t *testing.T) {
	producer := &mockProducer{}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	if emitter.EventType() != constants.HubHAResourcesMsgKey {
		t.Errorf("Expected EventType %s, got %s", constants.HubHAResourcesMsgKey, emitter.EventType())
	}
}

func TestHubHAEmitter_Update_SendsImmediately(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	// Create an unstructured Secret with required label
	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "test-secret",
				"namespace": "default",
				"labels": map[string]any{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			"data": map[string]any{
				"kubeconfig": "dGVzdA==",
			},
		},
	}

	// Update should send immediately
	err := emitter.Update(secret)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	// Verify event was sent
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent immediately, got %d", len(producer.events))
	}

	// Verify event content
	event := producer.events[0]
	if event.Type() != constants.HubHAResourcesMsgKey {
		t.Errorf("Expected event type %s, got %s", constants.HubHAResourcesMsgKey, event.Type())
	}

	// Parse bundle from event data
	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(event.Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	if len(bundle.Update) != 1 {
		t.Errorf("Expected 1 update in bundle, got %d", len(bundle.Update))
	}

	if bundle.Update[0].GetName() != "test-secret" {
		t.Errorf("Expected secret name 'test-secret', got %s", bundle.Update[0].GetName())
	}

	// Bundle should be cleared after send
	if len(emitter.bundle.Update) != 0 {
		t.Errorf("Expected bundle to be cleared after send, got %d updates", len(emitter.bundle.Update))
	}
}

func TestHubHAEmitter_Delete_SendsImmediately(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	// Create an unstructured Secret with required label
	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "test-secret",
				"namespace": "default",
				"labels": map[string]any{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}

	// Delete should send immediately
	err := emitter.Delete(secret)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify event was sent
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent immediately, got %d", len(producer.events))
	}

	// Parse bundle from event data
	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(producer.events[0].Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	if len(bundle.Delete) != 1 {
		t.Errorf("Expected 1 delete in bundle, got %d", len(bundle.Delete))
	}

	if bundle.Delete[0].Name != "test-secret" {
		t.Errorf("Expected secret name 'test-secret', got %s", bundle.Delete[0].Name)
	}
}

func TestHubHAEmitter_Update_FilteredResource(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	// Create a Secret WITHOUT required label
	// Note: Filtering is done by the predicate, not by Update() method
	// This test verifies that Update() sends whatever it receives
	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "test-secret",
				"namespace": "default",
			},
		},
	}

	// Update() will send it (filtering is predicate's responsibility)
	err := emitter.Update(secret)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	// Verify event was sent (predicate would have filtered it before calling Update)
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent, got %d", len(producer.events))
	}
}

func TestHubHAEmitter_Delete_WithoutLabels(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	// Create a Secret WITH required label - simulating a deleted object that was synced
	// The delete event contains the object's last known state with labels
	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "test-secret",
				"namespace": "default",
				"labels": map[string]any{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}

	// Delete should send because object has required labels (was synced)
	// This tests direct Delete() call (predicate is bypassed in unit tests)
	err := emitter.Delete(secret)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	// Verify event was sent
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent for deletion, got %d", len(producer.events))
	}

	// Parse bundle from event data
	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(producer.events[0].Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	if len(bundle.Delete) != 1 {
		t.Errorf("Expected 1 delete in bundle, got %d", len(bundle.Delete))
	}

	if bundle.Delete[0].Name != "test-secret" {
		t.Errorf("Expected secret name 'test-secret', got %s", bundle.Delete[0].Name)
	}
}

func TestHubHAEmitter_Resync(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	// Create test secrets
	secret1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret1",
				"namespace": "default",
				"labels": map[string]any{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}

	secret2 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret2",
				"namespace": "default",
				"labels": map[string]any{
					"cluster.open-cluster-management.io/type": "managed",
				},
			},
		},
	}

	// Resync should NOT send immediately
	err := emitter.Resync([]client.Object{secret1, secret2})
	if err != nil {
		t.Errorf("Resync() error = %v", err)
	}

	// No events should be sent yet
	if len(producer.events) != 0 {
		t.Errorf("Expected 0 events after Resync, got %d", len(producer.events))
	}

	// Now send the bundle
	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	// Verify event was sent
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event after Send, got %d", len(producer.events))
	}

	// Parse bundle
	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(producer.events[0].Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	if len(bundle.Resync) != 2 {
		t.Errorf("Expected 2 resync items in sent bundle, got %d", len(bundle.Resync))
	}
}

func TestHubHAEmitter_DisabledDropsEvents(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec-topic"},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	// emitter starts disabled — no SetEnabled(true)

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret",
				"namespace": "default",
				"labels":    map[string]any{"hive.openshift.io/secret-type": "kubeconfig"},
			},
		},
	}

	if err := emitter.Update(secret); err != nil {
		t.Fatalf("Update() unexpected error: %v", err)
	}
	if err := emitter.Delete(secret); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if err := emitter.Send(); err != nil {
		t.Fatalf("Send() unexpected error: %v", err)
	}
	if len(producer.events) != 0 {
		t.Errorf("expected 0 events when disabled, got %d", len(producer.events))
	}
}

func TestHubHAEmitter_SetEnabled_TogglesState(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec-topic"},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret",
				"namespace": "default",
				"labels":    map[string]any{"hive.openshift.io/secret-type": "kubeconfig"},
			},
		},
	}

	// Disabled: no events
	if err := emitter.Update(secret); err != nil {
		t.Fatalf("unexpected error from disabled Update: %v", err)
	}
	if len(producer.events) != 0 {
		t.Fatalf("expected 0 events while disabled, got %d", len(producer.events))
	}

	// Enable: events flow
	emitter.SetEnabled(true)
	if err := emitter.Update(secret); err != nil {
		t.Fatalf("unexpected error from enabled Update: %v", err)
	}
	if len(producer.events) != 1 {
		t.Fatalf("expected 1 event after enabling, got %d", len(producer.events))
	}

	// Disable again: no more events
	emitter.SetEnabled(false)
	if err := emitter.Update(secret); err != nil {
		t.Fatalf("unexpected error from disabled Update: %v", err)
	}
	if len(producer.events) != 1 {
		t.Errorf("expected no new events after disabling, got %d", len(producer.events))
	}
}

func TestHubHAEmitter_SetStandbyHub(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec-topic"},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)

	emitter.SetStandbyHub("hub3")

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret",
				"namespace": "default",
				"labels":    map[string]any{"hive.openshift.io/secret-type": "kubeconfig"},
			},
		},
	}
	if err := emitter.Update(secret); err != nil {
		t.Fatalf("unexpected error from Update: %v", err)
	}

	if len(producer.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(producer.events))
	}
	// ToCloudEvent sets the new standby hub as the CloudEvent Subject.
	if got := producer.events[0].Subject(); got != "hub3" {
		t.Errorf("expected subject=hub3, got %v", got)
	}
}

func TestHubHAEmitter_SelfList_NilClient(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec-topic"},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	emitter.SetEnabled(true)
	// No SetClient call — Resync(nil) should return an error

	err := emitter.Resync(nil)
	if err == nil {
		t.Error("expected error from Resync(nil) without client, got nil")
	}
}

func TestHubHAEmitter_Resync_WhenDisabled_IsNoop(t *testing.T) {
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec-topic"},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")
	// disabled

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "secret",
				"namespace": "default",
				"labels":    map[string]any{"hive.openshift.io/secret-type": "kubeconfig"},
			},
		},
	}
	if err := emitter.Resync([]client.Object{secret}); err != nil {
		t.Fatalf("Resync() error when disabled: %v", err)
	}
	if err := emitter.Send(); err != nil {
		t.Fatalf("Send() error: %v", err)
	}
	if len(producer.events) != 0 {
		t.Errorf("expected 0 events when disabled, got %d", len(producer.events))
	}
}

func TestCleanUnstructuredMetadata(t *testing.T) {
	uObj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":            "test",
				"namespace":       "default",
				"resourceVersion": "12345",
				"generation":      int64(5),
				"uid":             "abc-123",
				"managedFields": []any{
					map[string]any{"manager": "test"},
				},
			},
		},
	}

	cleanUnstructuredMetadata(uObj)

	if uObj.GetResourceVersion() != "" {
		t.Errorf("Expected resourceVersion to be cleared, got %s", uObj.GetResourceVersion())
	}

	if uObj.GetGeneration() != 0 {
		t.Errorf("Expected generation to be 0, got %d", uObj.GetGeneration())
	}

	if string(uObj.GetUID()) != "" {
		t.Errorf("Expected uid to be cleared, got %s", uObj.GetUID())
	}

	if uObj.GetManagedFields() != nil {
		t.Errorf("Expected managedFields to be nil, got %v", uObj.GetManagedFields())
	}

	// Name and namespace should remain
	if uObj.GetName() != "test" {
		t.Errorf("Expected name to remain, got %s", uObj.GetName())
	}

	if uObj.GetNamespace() != "default" {
		t.Errorf("Expected namespace to remain, got %s", uObj.GetNamespace())
	}
}

// ----- additional coverage tests -----

// errProducer always returns an error from SendEvent.
type errProducer struct{}

func (e *errProducer) SendEvent(_ context.Context, _ cloudevents.Event) error {
	return fmt.Errorf("deliberate send error")
}

func (e *errProducer) Stop() {}

func (e *errProducer) Reconnect(_ *transport.TransportInternalConfig, _ string) error { return nil }

// listErrorClient stubs only the List method so selfList receives an error.
type listErrorClient struct {
	client.Client
	err error
}

func (l *listErrorClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	return l.err
}

func newTransportConfig() *transport.TransportInternalConfig {
	return &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{SpecTopic: "spec"},
	}
}

// TestHubHAEmitter_SetActiveResources stores the GVK list.
func TestHubHAEmitter_SetActiveResources(t *testing.T) {
	emitter := NewHubHAEmitter(nil, nil, "hub1", "hub2")
	gvks := []schema.GroupVersionKind{
		{Group: "apps", Version: "v1", Kind: "Deployment"},
	}
	emitter.SetActiveResources(gvks)
	emitter.mu.Lock()
	got := len(emitter.activeResources)
	emitter.mu.Unlock()
	if got != 1 {
		t.Errorf("expected 1 activeResource after SetActiveResources, got %d", got)
	}
}

// TestHubHAEmitter_SetClient stores the Kubernetes client.
func TestHubHAEmitter_SetClient(t *testing.T) {
	emitter := NewHubHAEmitter(nil, nil, "hub1", "hub2")
	fc := &listErrorClient{}
	emitter.SetClient(fc)
	emitter.mu.Lock()
	got := emitter.client
	emitter.mu.Unlock()
	if got == nil {
		t.Error("expected non-nil client after SetClient")
	}
}

// TestHubHAEmitter_Send_EmptyBundle_ReturnsNil verifies that Send is a no-op
// when the bundle has no pending events.
func TestHubHAEmitter_Send_EmptyBundle_ReturnsNil(t *testing.T) {
	emitter := NewHubHAEmitter(&mockProducer{}, newTransportConfig(), "hub1", "hub2")
	emitter.SetEnabled(true)
	if err := emitter.Send(); err != nil {
		t.Errorf("expected nil error for empty bundle, got %v", err)
	}
}

// TestHubHAEmitter_Update_SendEventError propagates a SendEvent failure.
func TestHubHAEmitter_Update_SendEventError(t *testing.T) {
	emitter := NewHubHAEmitter(&errProducer{}, newTransportConfig(), "hub1", "hub2")
	emitter.SetEnabled(true)

	secret := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      "fail-secret",
				"namespace": "default",
				"labels": map[string]any{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}
	err := emitter.Update(secret)
	if err == nil {
		t.Error("expected error from failing producer, got nil")
	}
}

// TestHubHAEmitter_Delete_RemovesExistingUpdate verifies that a Delete for an
// object already queued in the Update list removes it from that list.
func TestHubHAEmitter_Delete_RemovesExistingUpdate(t *testing.T) {
	emitter := NewHubHAEmitter(&mockProducer{}, newTransportConfig(), "hub1", "hub2")
	emitter.SetEnabled(true)

	// Directly populate the Update bundle (bypasses send so the item stays in-flight).
	pending := &unstructured.Unstructured{}
	pending.SetAPIVersion("v1")
	pending.SetKind("Secret")
	pending.SetNamespace("default")
	pending.SetName("my-secret")
	emitter.bundle.Update = append(emitter.bundle.Update, pending)

	toDelete := &unstructured.Unstructured{}
	toDelete.SetAPIVersion("v1")
	toDelete.SetKind("Secret")
	toDelete.SetNamespace("default")
	toDelete.SetName("my-secret")

	if err := emitter.Delete(toDelete); err != nil {
		t.Fatalf("Delete() returned unexpected error: %v", err)
	}
	// After Delete the Update list should be empty.
	if len(emitter.bundle.Update) != 0 {
		t.Errorf("expected empty Update list after Delete, got %d items", len(emitter.bundle.Update))
	}
}

// TestHubHAEmitter_SelfList_NilClient_ReturnsError returns an error when the
// client argument passed to selfList is nil.
func TestHubHAEmitter_SelfList_NilClient_ReturnsError(t *testing.T) {
	emitter := NewHubHAEmitter(nil, nil, "hub1", "hub2")
	gvks := []schema.GroupVersionKind{{Group: "", Version: "v1", Kind: "Secret"}}
	_, err := emitter.selfList(context.Background(), nil, gvks)
	if err == nil {
		t.Error("expected error for nil client, got nil")
	}
}

// TestHubHAEmitter_SelfList_ListError collects non-NoMatchError list failures.
func TestHubHAEmitter_SelfList_ListError(t *testing.T) {
	emitter := NewHubHAEmitter(nil, nil, "hub1", "hub2")
	cl := &listErrorClient{err: fmt.Errorf("connection refused")}
	gvks := []schema.GroupVersionKind{{Group: "apps", Version: "v1", Kind: "Deployment"}}
	_, err := emitter.selfList(context.Background(), cl, gvks)
	if err == nil {
		t.Error("expected error from list failure, got nil")
	}
}

// TestHubHAEmitter_ToUnstructured_TypedObject converts a concrete typed object
// via the JSON marshal path (non-Unstructured branch).
func TestHubHAEmitter_ToUnstructured_TypedObject(t *testing.T) {
	cm := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "typed-cm",
			Namespace: "default",
		},
	}
	uObj, err := toUnstructured(cm)
	if err != nil {
		t.Fatalf("toUnstructured() error = %v", err)
	}
	if uObj.GetName() != "typed-cm" {
		t.Errorf("expected name 'typed-cm', got %s", uObj.GetName())
	}
}
