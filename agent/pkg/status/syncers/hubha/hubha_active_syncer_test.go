// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"encoding/json"
	"testing"

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
	resources := GetHubHAResourcesToSync()

	if len(resources) == 0 {
		t.Error("GetHubHAResourcesToSync() returned empty list")
	}

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
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

	controller := &hubHAResourceSyncer{
		client:  fakeClient,
		emitter: emitter,
	}
	controller.enabled.Store(true)

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
				"kubeconfig": "dGVzdA==",
			},
		},
	}

	err := fakeClient.Create(context.Background(), secret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	encodedName := "||v1||Secret||test-secret"
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

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent, got %d", len(producer.events))
	}
}

func TestHubHAController_Reconcile_Delete(t *testing.T) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

	controller := &hubHAResourceSyncer{
		client:  fakeClient,
		emitter: emitter,
	}
	controller.enabled.Store(true)

	encodedName := "||v1||Secret||deleted-secret"
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
}

func TestHubHAController_Reconcile_FilteredResource(t *testing.T) {
	scheme := newTestScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", nil)

	controller := &hubHAResourceSyncer{
		client:  fakeClient,
		emitter: emitter,
	}
	controller.enabled.Store(true)

	secret := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name":      "filtered-secret",
				"namespace": "default",
			},
		},
	}

	err := fakeClient.Create(context.Background(), secret)
	if err != nil {
		t.Fatalf("Failed to create secret: %v", err)
	}

	encodedName := "||v1||Secret||filtered-secret"
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

	err = emitter.Send()
	if err != nil {
		t.Errorf("Send() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event when calling Reconcile directly, got %d", len(producer.events))
	}
}

func TestPerformFullResync(t *testing.T) {
	scheme := newTestScheme()

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

	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}
	emitter := NewHubHAEmitter(producer, transportConfig, "hub1", "hub2", fakeClient)
	emitter.SetActiveResources([]schema.GroupVersionKind{
		{Group: "", Version: "v1", Kind: "Secret"},
	})

	err := emitter.Resync(nil)
	if err != nil {
		t.Errorf("Resync() error = %v", err)
	}

	if len(producer.events) != 2 {
		t.Fatalf("Expected 2 events to be sent after resync, got %d", len(producer.events))
	}

	var bundle generic.GenericBundle[*unstructured.Unstructured]
	err = json.Unmarshal(producer.events[0].Data(), &bundle)
	if err != nil {
		t.Errorf("Failed to unmarshal bundle: %v", err)
	}

	if len(bundle.Resync) != 2 {
		t.Errorf("Expected 2 secrets in resync bundle, got %d", len(bundle.Resync))
	}
}

func newTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return scheme
}
