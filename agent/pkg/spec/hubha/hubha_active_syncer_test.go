// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGetHubHAResourcesToSync(t *testing.T) {
	resources := getHubHAResourcesToSync()

	// Verify we have resources to sync
	if len(resources) == 0 {
		t.Error("getHubHAResourcesToSync() returned empty list")
	}

	// Verify some key resource types are included
	expectedResources := []schema.GroupVersionKind{
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy"},
		{Group: "cluster.open-cluster-management.io", Version: "v1beta1", Kind: "Placement"},
		{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeployment"},
		{Group: "argoproj.io", Version: "v1alpha1", Kind: "Application"},
		{Group: "", Version: "v1", Kind: "Secret"},
		{Group: "", Version: "v1", Kind: "ConfigMap"},
	}

	for _, expected := range expectedResources {
		found := false
		for _, gvk := range resources {
			if gvk.Group == expected.Group && gvk.Kind == expected.Kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected resource %s not found in Hub HA resources list", expected.String())
		}
	}
}

func TestHubHAController_Reconcile_Update(t *testing.T) {
	// Create fake client
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create mock producer and emitter
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	// Create controller
	controller := &hubHAController{
		client:  fakeClient,
		emitter: emitter,
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
	}

	// Create a Secret with required label
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "test-secret",
				"namespace": "default",
				"labels": map[string]interface{}{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			"data": map[string]interface{}{
				"kubeconfig": "dGVzdA==", // base64 "test"
			},
		},
	}

	// Create the secret in fake client
	err := fakeClient.Create(context.Background(), secret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-secret",
		},
	}

	result, err := controller.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue")
	}

	// Verify event was sent immediately
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent, got %d", len(producer.events))
	}
}

func TestHubHAController_Reconcile_Delete(t *testing.T) {
	// Create fake client
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create mock producer and emitter
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	// Create controller
	controller := &hubHAController{
		client:  fakeClient,
		emitter: emitter,
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
	}

	// Reconcile for non-existent object (deleted)
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "deleted-secret",
		},
	}

	result, err := controller.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue for deleted object")
	}

	// Note: Delete for non-existent object without required label won't send event
	// because the filter will reject it
}

func TestHubHAController_Reconcile_FilteredResource(t *testing.T) {
	// Create fake client
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create mock producer and emitter
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	// Create controller
	controller := &hubHAController{
		client:  fakeClient,
		emitter: emitter,
		gvk: schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Secret",
		},
	}

	// Create a Secret WITHOUT required label (should be filtered)
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "filtered-secret",
				"namespace": "default",
				// No required labels
			},
		},
	}

	// Create the secret in fake client
	err := fakeClient.Create(context.Background(), secret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	// Reconcile
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "filtered-secret",
		},
	}

	result, err := controller.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue")
	}

	// Verify no event was sent (filtered)
	if len(producer.events) != 0 {
		t.Errorf("Expected 0 events for filtered resource, got %d", len(producer.events))
	}
}

func TestPerformFullResync(t *testing.T) {
	// Create fake client
	scheme := newTestScheme()

	// Create test secrets
	secret1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "secret1",
				"namespace": "default",
				"labels": map[string]interface{}{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}

	secret2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "secret2",
				"namespace": "default",
				"labels": map[string]interface{}{
					"cluster.open-cluster-management.io/type": "managed",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret1, secret2).
		Build()

	// Create mock producer and emitter
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	// Perform resync
	resourcesToSync := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
	}

	err := performFullResync(context.Background(), fakeClient, emitter, resourcesToSync)
	if err != nil {
		t.Errorf("performFullResync() error = %v", err)
	}

	// Verify event was sent
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent after resync, got %d", len(producer.events))
	}

	// Parse bundle
	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(producer.events[0].Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	// Should have 2 secrets in resync
	if len(bundle.Resync) != 2 {
		t.Errorf("Expected 2 secrets in resync bundle, got %d", len(bundle.Resync))
	}
}

func TestPeriodicResync(t *testing.T) {
	// Create fake client
	scheme := newTestScheme()
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "test-secret",
				"namespace": "default",
				"labels": map[string]interface{}{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	// Create mock producer and emitter
	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2")

	resourcesToSync := []schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
	}

	// Start periodic resync with very short interval for testing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use very short interval for testing
	interval := 100 * time.Millisecond
	go periodicResync(ctx, fakeClient, emitter, resourcesToSync, interval)

	// Wait for at least one resync cycle
	time.Sleep(200 * time.Millisecond)

	// Cancel to stop goroutine
	cancel()

	// Verify at least one resync event was sent
	if len(producer.events) < 1 {
		t.Errorf("Expected at least 1 resync event, got %d", len(producer.events))
	}
}

// Helper function to create a test scheme
func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}
