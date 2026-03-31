// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	// Test deletion of resources using GVK information in ObjectMetadata
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	// Create a ConfigMap to be deleted
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	syncer := NewHubHAStandbySyncer(client)

	meta := &generic.ObjectMetadata{
		Name:      "test-cm",
		Namespace: "default",
		ID:        "test-id",
		Group:     "",
		Version:   "v1",
		Kind:      "ConfigMap",
	}

	ctx := context.Background()
	err := syncer.deleteResource(ctx, meta, "hub1")
	if err != nil {
		t.Errorf("deleteResource() error = %v", err)
	}

	// Verify the resource was actually deleted
	verifyDeleted := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, verifyDeleted)
	if err == nil {
		t.Errorf("expected resource to be deleted, but it still exists")
	}
	if !errors.IsNotFound(err) {
		t.Errorf("expected NotFound error, got %v", err)
	}
}

func TestHubHAStandbySyncer_UpdateResource_CleansOwnerReferences(t *testing.T) {
	// Test that ownerReferences are cleaned to avoid permission issues
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	syncer := NewHubHAStandbySyncer(client)

	// Create resource with ownerReferences
	cmWithOwner := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "test-cm",
				"namespace": "default",
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "v1",
						"kind":               "Pod",
						"name":               "owner-pod",
						"uid":                "12345",
						"blockOwnerDeletion": true,
					},
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}

	ctx := context.Background()
	err := syncer.updateResource(ctx, cmWithOwner, "hub1")
	if err != nil {
		t.Errorf("updateResource() error = %v", err)
	}

	// Verify resource was created without ownerReferences
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(cmWithOwner.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: "test-cm", Namespace: "default"}, result)
	if err != nil {
		t.Errorf("Failed to get resource: %v", err)
	}

	ownerRefs := result.GetOwnerReferences()
	if len(ownerRefs) != 0 {
		t.Errorf("Expected ownerReferences to be cleaned, but got %v", ownerRefs)
	}
}

func TestHubHAStandbySyncer_CreateManagedCluster_SetsHubAcceptsClient(t *testing.T) {
	// Test that when creating ManagedCluster via Hub HA sync,
	// hubAcceptsClient is set to false (standby should not accept clients)
	scheme := runtime.NewScheme()

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	syncer := NewHubHAStandbySyncer(client)

	// Create ManagedCluster resource
	managedCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true, // Should be overridden to false
			},
		},
	}

	ctx := context.Background()
	err := syncer.createResource(ctx, managedCluster, "hub1")
	if err != nil {
		t.Errorf("createResource() error = %v", err)
	}

	// Verify ManagedCluster was created with hubAcceptsClient=false
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(managedCluster.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: "cluster1"}, result)
	if err != nil {
		t.Errorf("Failed to get created ManagedCluster: %v", err)
	}

	hubAcceptsClient, found, err := unstructured.NestedBool(result.Object, "spec", "hubAcceptsClient")
	if err != nil || !found {
		t.Errorf("Failed to get spec.hubAcceptsClient from ManagedCluster")
	}
	if hubAcceptsClient != false {
		t.Errorf("ManagedCluster hubAcceptsClient = %v, want false", hubAcceptsClient)
	}
}

func TestHubHAStandbySyncer_UpdateManagedCluster_SetsHubAcceptsClient(t *testing.T) {
	// Test that when updating ManagedCluster via Hub HA sync,
	// hubAcceptsClient is set to false (standby should not accept clients)
	scheme := runtime.NewScheme()

	// Create existing ManagedCluster with hubAcceptsClient=true
	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(existing).
		Build()

	syncer := NewHubHAStandbySyncer(client)

	// Update with new labels but hubAcceptsClient still true
	updated := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
				"labels": map[string]interface{}{
					"new-label": "new-value",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true, // Should be overridden to false
			},
		},
	}

	ctx := context.Background()
	err := syncer.updateResource(ctx, updated, "hub1")
	if err != nil {
		t.Errorf("updateResource() error = %v", err)
	}

	// Verify ManagedCluster was updated with hubAcceptsClient=false
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(updated.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: "cluster1"}, result)
	if err != nil {
		t.Errorf("Failed to get updated ManagedCluster: %v", err)
	}

	hubAcceptsClient, found, err := unstructured.NestedBool(result.Object, "spec", "hubAcceptsClient")
	if err != nil || !found {
		t.Errorf("Failed to get spec.hubAcceptsClient from ManagedCluster")
	}
	if hubAcceptsClient != false {
		t.Errorf("ManagedCluster hubAcceptsClient = %v, want false", hubAcceptsClient)
	}

	// Verify labels were still updated
	labels, found, err := unstructured.NestedStringMap(result.Object, "metadata", "labels")
	if err != nil || !found {
		t.Errorf("Failed to get labels from ManagedCluster")
	}
	if labels["new-label"] != "new-value" {
		t.Errorf("Labels not updated correctly, got %v", labels)
	}
}
