// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGetHubHAResourcesToSync(t *testing.T) {
	resources := GetHubHAResourcesToSync()

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
	emitter.SetEnabled(true) // hub is in active role

	// Create controller
	controller := &hubHAController{
		client:  fakeClient,
		emitter: emitter,
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

	// Reconcile with encoded name (GVK encoding format)
	encodedName := "||v1||Secret||test-secret" // Empty group for core/v1 resources
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      encodedName,
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
	}

	// Reconcile for non-existent object (deleted) with encoded name
	encodedName := "||v1||Secret||deleted-secret" // Empty group for core/v1 resources
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      encodedName,
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
	emitter.SetEnabled(true) // hub is in active role

	// Create controller
	controller := &hubHAController{
		client:  fakeClient,
		emitter: emitter,
	}

	// Create a Secret WITHOUT required label
	// Note: In production, the predicate would filter this before Reconcile is called.
	// This test directly calls Reconcile (bypassing predicate), so it will send the event.
	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "filtered-secret",
				"namespace": "default",
				// No required labels - predicate would filter this in production
			},
		},
	}

	// Create the secret in fake client
	err := fakeClient.Create(context.Background(), secret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	// Reconcile with encoded name
	encodedName := "||v1||Secret||filtered-secret" // Empty group for core/v1 resources
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      encodedName,
		},
	}

	result, err := controller.Reconcile(context.Background(), req)
	if err != nil {
		t.Errorf("Reconcile() error = %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue")
	}

	// When Reconcile is called directly (bypassing predicate), it will send the event
	// In production, predicate filters before Reconcile is called
	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event when calling Reconcile directly, got %d", len(producer.events))
	}
}

// Helper function to create a test scheme
func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}
