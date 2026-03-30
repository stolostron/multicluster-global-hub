// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
