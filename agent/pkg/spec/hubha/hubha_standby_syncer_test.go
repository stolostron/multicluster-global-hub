// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestNewHubHAStandbySyncer(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := NewHubHAStandbySyncer(client)

	if syncer == nil {
		t.Error("NewHubHAStandbySyncer() returned nil")
	} else {
		if syncer.client == nil {
			t.Error("HubHAStandbySyncer client is nil")
		}
	}
}

func TestHubHAStandbySyncer_Sync_WrongEventType(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := NewHubHAStandbySyncer(client)

	// Create event with wrong type
	evt := cloudevents.NewEvent()
	evt.SetType("WrongType")
	evt.SetSource("hub1")

	ctx := context.Background()
	err := syncer.Sync(ctx, &evt)
	// Should not return error for wrong event type, just ignore it
	if err != nil {
		t.Errorf("Sync() with wrong event type should not error, got: %v", err)
	}
}

func TestHubHAStandbySyncer_CreateResource(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := NewHubHAStandbySyncer(client)

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	ctx := context.Background()
	err := syncer.createResource(ctx, obj, "hub1")
	if err != nil {
		t.Errorf("createResource() error = %v", err)
	}

	// Verify resource was created
	created := &unstructured.Unstructured{}
	created.SetGroupVersionKind(obj.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, created)
	if err != nil {
		t.Errorf("Failed to get created resource: %v", err)
	}

	if created.GetName() != "test-cm" {
		t.Errorf("Created resource name = %s, want test-cm", created.GetName())
	}
}

func TestHubHAStandbySyncer_UpdateResource(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create existing resource
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "old-value",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	syncer := NewHubHAStandbySyncer(client)

	// Update with new data
	updated := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "new-value",
			},
		},
	}

	ctx := context.Background()
	err := syncer.updateResource(ctx, updated, "hub1")
	if err != nil {
		t.Errorf("updateResource() error = %v", err)
	}

	// Verify resource was updated
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(updated.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: updated.GetName(), Namespace: updated.GetNamespace()}, result)
	if err != nil {
		t.Errorf("Failed to get updated resource: %v", err)
	}

	data, found, err := unstructured.NestedString(result.Object, "data", "key")
	if err != nil || !found {
		t.Errorf("Failed to get data.key from updated resource")
	}
	if data != "new-value" {
		t.Errorf("Updated resource data = %s, want new-value", data)
	}
}

func TestHubHAStandbySyncer_UpdateResource_CreateIfNotExists(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := NewHubHAStandbySyncer(client)

	// Try to update a non-existent resource
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	ctx := context.Background()
	err := syncer.updateResource(ctx, obj, "hub1")
	if err != nil {
		t.Errorf("updateResource() error = %v", err)
	}

	// Verify resource was created
	created := &unstructured.Unstructured{}
	created.SetGroupVersionKind(obj.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, created)
	if err != nil {
		t.Errorf("Failed to get created resource: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_FullBundle(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create existing resource to be updated
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "existing-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": "old-value",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	syncer := NewHubHAStandbySyncer(client)

	// Create bundle with create, update, and resync
	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()

	// Add create
	bundle.Create = []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "new-cm",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		},
	}

	// Add update
	bundle.Update = []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "existing-cm",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "updated-value",
				},
			},
		},
	}

	// Add resync
	bundle.Resync = []*unstructured.Unstructured{
		{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "resync-cm",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "resync-value",
				},
			},
		},
	}

	// Create CloudEvent
	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Errorf("SetData() error = %v", err)
	}

	ctx := context.Background()
	if err := syncer.Sync(ctx, &evt); err != nil {
		t.Errorf("Sync() error = %v", err)
	}

	// Verify created resource
	newCM := &unstructured.Unstructured{}
	newCM.SetGroupVersionKind(bundle.Create[0].GroupVersionKind())
	if err := client.Get(ctx, types.NamespacedName{Name: "new-cm", Namespace: "default"}, newCM); err != nil {
		t.Errorf("Failed to get created resource: %v", err)
	}

	// Verify updated resource
	updatedCM := &unstructured.Unstructured{}
	updatedCM.SetGroupVersionKind(bundle.Update[0].GroupVersionKind())
	if err := client.Get(ctx, types.NamespacedName{Name: "existing-cm", Namespace: "default"}, updatedCM); err != nil {
		t.Errorf("Failed to get updated resource: %v", err)
	}
	data, _, _ := unstructured.NestedString(updatedCM.Object, "data", "key")
	if data != "updated-value" {
		t.Errorf("Updated resource data = %s, want updated-value", data)
	}

	// Verify resynced resource
	resyncCM := &unstructured.Unstructured{}
	resyncCM.SetGroupVersionKind(bundle.Resync[0].GroupVersionKind())
	if err := client.Get(ctx, types.NamespacedName{Name: "resync-cm", Namespace: "default"}, resyncCM); err != nil {
		t.Errorf("Failed to get resynced resource: %v", err)
	}
}

func TestHubHAStandbySyncer_DeleteResource(t *testing.T) {
	t.Skip("deleteResource is not yet implemented - resources deleted on active hub remain on standby hub. " +
		"This test should verify actual deletion once Hub HA delete/resync pruning is implemented (needs GVK in ObjectMetadata)")

	// TODO: Once deletion is implemented, this test should:
	// 1. Create a resource on the fake client
	// 2. Call deleteResource with proper GVK information
	// 3. Verify the resource is actually deleted from the client
	// 4. Ensure resources removed from active hub don't remain stranded on standby hub

	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := NewHubHAStandbySyncer(client)

	meta := &generic.ObjectMetadata{
		Name:      "test-cm",
		Namespace: "default",
		ID:        "test-id",
	}

	ctx := context.Background()
	err := syncer.deleteResource(ctx, meta, "hub1")
	if err != nil {
		t.Errorf("deleteResource() error = %v", err)
	}
}
