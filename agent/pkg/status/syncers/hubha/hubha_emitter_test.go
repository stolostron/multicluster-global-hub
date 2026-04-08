// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

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

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

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

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

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

	err := emitter.Update(secret)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent after Send, got %d", len(producer.events))
	}

	event := producer.events[0]
	if event.Type() != constants.HubHAResourcesMsgKey {
		t.Errorf("Expected event type %s, got %s", constants.HubHAResourcesMsgKey, event.Type())
	}

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

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

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

	err := emitter.Delete(secret)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent after Send, got %d", len(producer.events))
	}

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

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

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

	err := emitter.Update(secret)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

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

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

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

	err := emitter.Delete(secret)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent for deletion, got %d", len(producer.events))
	}

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

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	secret1 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret1",
			Namespace: "default",
			Labels: map[string]string{
				"hive.openshift.io/secret-type": "kubeconfig",
			},
		},
	}
	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "secret2",
			Namespace: "default",
			Labels: map[string]string{
				"cluster.open-cluster-management.io/type": "managed",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret1, secret2).
		Build()

	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", fakeClient)
	emitter.SetActiveResources([]schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
	})

	err := emitter.Resync(nil)
	if err != nil {
		t.Fatalf("Resync() error = %v", err)
	}

	// Expect 2 events: one with resync objects, one with resync metadata
	if len(producer.events) != 2 {
		t.Fatalf("Expected 2 events after Resync, got %d", len(producer.events))
	}

	var resyncBundle generic.GenericBundle[*unstructured.Unstructured]
	if err := json.Unmarshal(producer.events[0].Data(), &resyncBundle); err != nil {
		t.Fatalf("Failed to unmarshal resync bundle: %v", err)
	}
	if len(resyncBundle.Resync) != 2 {
		t.Errorf("Expected 2 resync items, got %d", len(resyncBundle.Resync))
	}

	var metaBundle generic.GenericBundle[*unstructured.Unstructured]
	if err := json.Unmarshal(producer.events[1].Data(), &metaBundle); err != nil {
		t.Fatalf("Failed to unmarshal metadata bundle: %v", err)
	}
	if len(metaBundle.ResyncMetadata) != 2 {
		t.Errorf("Expected 2 resync metadata items, got %d", len(metaBundle.ResyncMetadata))
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

	if uObj.GetName() != "test" {
		t.Errorf("Expected name to remain, got %s", uObj.GetName())
	}

	if uObj.GetNamespace() != "default" {
		t.Errorf("Expected namespace to remain, got %s", uObj.GetNamespace())
	}
}
