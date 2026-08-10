// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"fmt"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	testCMName                    = "test-cm"
	existingCMName                = "existing-cm"
	newValue                      = "new-value"
	managedClusterAPIVersion      = "cluster.open-cluster-management.io/v1"
	errFailedToGetCreatedResource = "Failed to get created resource: %v"
	errUpdateResource             = "updateResource() error = %v"
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

func TestHubHAStandbySyncer_Sync_RejectsInvalidBundle(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	syncer := NewHubHAStandbySyncer(client)
	ctx := context.Background()

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	_ = evt.SetData(cloudevents.ApplicationJSON, []byte("not-json"))

	if err := syncer.Sync(ctx, &evt); err == nil {
		t.Fatal("expected invalid bundle data to return an error")
	}
}

func TestHubHAStandbySyncer_DeleteResource_MissingKind(t *testing.T) {
	scheme := runtime.NewScheme()
	syncer := NewHubHAStandbySyncer(fake.NewClientBuilder().WithScheme(scheme).Build())

	meta := &generic.ObjectMetadata{
		Name:      testCMName,
		Namespace: "default",
	}
	err := syncer.deleteResource(context.Background(), meta, "hub1")
	if err == nil {
		t.Fatal("expected missing kind to return an error")
	}
}

func TestHubHAStandbySyncer_DeleteResource_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	syncer := NewHubHAStandbySyncer(fake.NewClientBuilder().WithScheme(scheme).Build())

	meta := &generic.ObjectMetadata{
		Name:      "missing-cm",
		Namespace: "default",
		Group:     "",
		Version:   "v1",
		Kind:      "ConfigMap",
	}
	if err := syncer.deleteResource(context.Background(), meta, "hub1"); err != nil {
		t.Fatalf("deleting missing resource should succeed, got %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_RejectsUntrustedSource(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	syncer := NewHubHAStandbySyncer(client)
	ctx := context.Background()

	emptySource := cloudevents.NewEvent()
	emptySource.SetType(constants.HubHAResourcesMsgKey)
	emptySource.SetSource("")
	if err := syncer.Sync(ctx, &emptySource); err != nil {
		t.Fatalf("Sync() with empty source should drop event without error, got: %v", err)
	}

	managerSource := cloudevents.NewEvent()
	managerSource.SetType(constants.HubHAResourcesMsgKey)
	managerSource.SetSource(constants.CloudEventGlobalHubClusterName)
	if err := syncer.Sync(ctx, &managerSource); err != nil {
		t.Fatalf("Sync() with global-hub source should drop event without error, got: %v", err)
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
				"name":      testCMName,
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
		t.Errorf(errFailedToGetCreatedResource, err)
	}

	if created.GetName() != testCMName {
		t.Errorf("Created resource name = %s, want %s", created.GetName(), testCMName)
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
				"name":      testCMName,
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
				"name":      testCMName,
				"namespace": "default",
			},
			"data": map[string]interface{}{
				"key": newValue,
			},
		},
	}

	ctx := context.Background()
	err := syncer.updateResource(ctx, updated, "hub1")
	if err != nil {
		t.Errorf(errUpdateResource, err)
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
	if data != newValue {
		t.Errorf("Updated resource data = %s, want %s", data, newValue)
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
				"name":      testCMName,
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
		t.Errorf(errUpdateResource, err)
	}

	// Verify resource was created
	created := &unstructured.Unstructured{}
	created.SetGroupVersionKind(obj.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}, created)
	if err != nil {
		t.Errorf(errFailedToGetCreatedResource, err)
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
				"name":      existingCMName,
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
					"name":      existingCMName,
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
	if err := client.Get(ctx, types.NamespacedName{Name: existingCMName, Namespace: "default"}, updatedCM); err != nil {
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
			Name:      testCMName,
			Namespace: "default",
		},
		Data: map[string]string{
			"key": "value",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cm).Build()
	syncer := NewHubHAStandbySyncer(client)

	meta := &generic.ObjectMetadata{
		Name:      testCMName,
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
	err = client.Get(ctx, types.NamespacedName{Name: testCMName, Namespace: "default"}, verifyDeleted)
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
				"name":      testCMName,
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
		t.Errorf(errUpdateResource, err)
	}

	// Verify resource was created without ownerReferences
	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(cmWithOwner.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: testCMName, Namespace: "default"}, result)
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
			"apiVersion": managedClusterAPIVersion,
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
			"apiVersion": managedClusterAPIVersion,
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
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "cluster1",
				"labels": map[string]interface{}{
					"new-label": newValue,
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
		t.Errorf(errUpdateResource, err)
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
	if labels["new-label"] != newValue {
		t.Errorf("Labels not updated correctly, got %v", labels)
	}
}

// Regression for e2e: ManagedCluster updates from active still carry status, and standby
// already has controller finalizers. Apply must succeed and sync labels/spec.
func TestHubHAStandbySyncer_UpdateManagedCluster_AppliesLabelsWithStatusAndFinalizers(t *testing.T) {
	scheme := runtime.NewScheme()

	existing := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "hubha-test-cluster",
				"labels": map[string]interface{}{
					"env": "test",
				},
				"finalizers": []interface{}{
					"cluster.open-cluster-management.io/api-resource-cleanup",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": false,
				"managedClusterClientConfigs": []interface{}{
					map[string]interface{}{
						"url": "https://test-cluster-v1.example.com:6443",
					},
				},
			},
			"status": map[string]interface{}{
				"version": map[string]interface{}{"kubernetes": "v1.28.0"},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	syncer := NewHubHAStandbySyncer(cl)

	updated := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "hubha-test-cluster",
				"labels": map[string]interface{}{
					"env": "production",
				},
			},
			"spec": map[string]interface{}{
				"hubAcceptsClient": true,
				"managedClusterClientConfigs": []interface{}{
					map[string]interface{}{
						"url": "https://test-cluster-v2.example.com:6443",
					},
				},
			},
			"status": map[string]interface{}{
				"version": map[string]interface{}{"kubernetes": "v1.29.0"},
			},
		},
	}

	ctx := context.Background()
	if err := syncer.updateResource(ctx, updated, "hub1"); err != nil {
		t.Fatalf(errUpdateResource, err)
	}

	result := &unstructured.Unstructured{}
	result.SetGroupVersionKind(updated.GroupVersionKind())
	if err := cl.Get(ctx, types.NamespacedName{Name: "hubha-test-cluster"}, result); err != nil {
		t.Fatalf("Failed to get updated ManagedCluster: %v", err)
	}

	if result.GetLabels()["env"] != "production" {
		t.Errorf("label env = %q, want production", result.GetLabels()["env"])
	}

	hubAcceptsClient, found, err := unstructured.NestedBool(result.Object, "spec", "hubAcceptsClient")
	if err != nil || !found || hubAcceptsClient {
		t.Errorf("hubAcceptsClient = %v (found=%v err=%v), want false", hubAcceptsClient, found, err)
	}

	configs, found, err := unstructured.NestedSlice(result.Object, "spec", "managedClusterClientConfigs")
	if err != nil || !found || len(configs) == 0 {
		t.Fatalf("Failed to get client configs: found=%v len=%d err=%v", found, len(configs), err)
	}
	cfg, ok := configs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("client config[0] type = %T, want map", configs[0])
	}
	if url, _ := cfg["url"].(string); url != "https://test-cluster-v2.example.com:6443" {
		t.Errorf("URL = %q, want v2 URL", url)
	}

	finalizers := result.GetFinalizers()
	if len(finalizers) != 1 || finalizers[0] != "cluster.open-cluster-management.io/api-resource-cleanup" {
		t.Errorf("finalizers = %v, want standby finalizer preserved", finalizers)
	}
}

func TestHubHAStandbySyncer_Sync_ReturnsAggregateErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	cmOne := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-one", Namespace: "default"},
	}
	cmTwo := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-two", Namespace: "default"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cmOne, cmTwo).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				switch obj.GetName() {
				case "cm-one":
					return fmt.Errorf("delete failure one")
				case "cm-two":
					return fmt.Errorf("delete failure two")
				default:
					return c.Delete(ctx, obj, opts...)
				}
			},
		}).
		Build()

	syncer := NewHubHAStandbySyncer(fakeClient)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.Delete = []generic.ObjectMetadata{
		{
			Name:      "cm-one",
			Namespace: "default",
			Group:     "",
			Version:   "v1",
			Kind:      "ConfigMap",
		},
		{
			Name:      "cm-two",
			Namespace: "default",
			Group:     "",
			Version:   "v1",
			Kind:      "ConfigMap",
		},
	}

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}

	err := syncer.Sync(context.Background(), &evt)
	assertSyncAggregateErrors(t, err, 2, "delete failure one", "delete failure two")
}

func hubHALabeledConfigMap(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"hive.openshift.io/secret-type": "kubeconfig",
				},
			},
			"data": map[string]interface{}{
				"key": "value",
			},
		},
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_CleansStaleResources(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	keep := hubHALabeledConfigMap("keep-cm", "default")
	stale := hubHALabeledConfigMap("stale-cm", "default")
	// Unlabeled ConfigMap should not be deleted by Hub HA cleanup.
	localOnly := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      "local-only-cm",
				"namespace": "default",
			},
			"data": map[string]interface{}{"key": "value"},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(keep, stale, localOnly).
		Build()
	syncer := NewHubHAStandbySyncer(cl)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.ResyncMetadata = []generic.ObjectMetadata{
		{Namespace: "default", Name: "keep-cm", Group: "", Version: "v1", Kind: "ConfigMap"},
	}

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}

	if err := syncer.Sync(context.Background(), &evt); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	ctx := context.Background()
	keepObj := &unstructured.Unstructured{}
	keepObj.SetAPIVersion("v1")
	keepObj.SetKind("ConfigMap")
	if err := cl.Get(ctx, types.NamespacedName{Name: "keep-cm", Namespace: "default"}, keepObj); err != nil {
		t.Fatalf("expected keep-cm to remain: %v", err)
	}

	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj)
	if err == nil {
		t.Fatal("expected stale-cm to be deleted")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for stale-cm, got: %v", err)
	}

	localObj := &unstructured.Unstructured{}
	localObj.SetAPIVersion("v1")
	localObj.SetKind("ConfigMap")
	if err := cl.Get(ctx, types.NamespacedName{Name: "local-only-cm", Namespace: "default"}, localObj); err != nil {
		t.Fatalf("expected unlabeled local-only-cm to remain: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_WaitsForComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	stale := hubHALabeledConfigMap("stale-cm", "default")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()
	syncer := NewHubHAStandbySyncer(cl)

	partial := generic.NewGenericBundle[*unstructured.Unstructured]()
	partial.ResyncMetadata = []generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", InventoryBegin: true},
		{Namespace: "default", Name: "keep-cm", Group: "", Version: "v1", Kind: "ConfigMap"},
	}
	partialEvt := cloudevents.NewEvent()
	partialEvt.SetType(constants.HubHAResourcesMsgKey)
	partialEvt.SetSource("hub1")
	if err := partialEvt.SetData(cloudevents.ApplicationJSON, partial); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	if err := syncer.Sync(context.Background(), &partialEvt); err != nil {
		t.Fatalf("Sync() partial error = %v", err)
	}

	ctx := context.Background()
	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	if err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj); err != nil {
		t.Fatalf("stale-cm should still exist before Complete, got: %v", err)
	}

	final := generic.NewGenericBundle[*unstructured.Unstructured]()
	final.ResyncMetadata = []generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	}
	finalEvt := cloudevents.NewEvent()
	finalEvt.SetType(constants.HubHAResourcesMsgKey)
	finalEvt.SetSource("hub1")
	if err := finalEvt.SetData(cloudevents.ApplicationJSON, final); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	if err := syncer.Sync(context.Background(), &finalEvt); err != nil {
		t.Fatalf("Sync() complete error = %v", err)
	}

	err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj)
	if err == nil {
		t.Fatal("expected stale-cm to be deleted after Complete")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for stale-cm, got: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_EmptyInventory_DeletesAll(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	stale := hubHALabeledConfigMap("stale-cm", "default")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()
	syncer := NewHubHAStandbySyncer(cl)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.ResyncMetadata = []generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", InventoryBegin: true},
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	}
	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	if err := syncer.Sync(context.Background(), &evt); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	err := cl.Get(context.Background(), types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj)
	if err == nil {
		t.Fatal("expected stale-cm to be deleted for empty inventory")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for stale-cm, got: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_BeginMiddleComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	keep1 := hubHALabeledConfigMap("keep-1", "default")
	keep2 := hubHALabeledConfigMap("keep-2", "default")
	stale := hubHALabeledConfigMap("stale-cm", "default")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(keep1, keep2, stale).Build()
	syncer := NewHubHAStandbySyncer(cl)

	syncMeta := func(metas []generic.ObjectMetadata) {
		t.Helper()
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.ResyncMetadata = metas
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
			t.Fatalf("SetData() error = %v", err)
		}
		if err := syncer.Sync(context.Background(), &evt); err != nil {
			t.Fatalf("Sync() error = %v", err)
		}
	}

	// Begin frame with first live object.
	syncMeta([]generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", InventoryBegin: true},
		{Namespace: "default", Name: "keep-1", Group: "", Version: "v1", Kind: "ConfigMap"},
	})
	// Markerless middle frame must append, not run legacy cleanup (which would delete keep-1).
	syncMeta([]generic.ObjectMetadata{
		{Namespace: "default", Name: "keep-2", Group: "", Version: "v1", Kind: "ConfigMap"},
	})
	syncMeta([]generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	})

	ctx := context.Background()
	for _, name := range []string{"keep-1", "keep-2"} {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("v1")
		obj.SetKind("ConfigMap")
		if err := cl.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, obj); err != nil {
			t.Fatalf("expected %s to remain after Begin/middle/Complete: %v", name, err)
		}
	}
	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj)
	if err == nil {
		t.Fatal("expected stale-cm to be deleted after Complete")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for stale-cm, got: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_RetryAfterCleanupFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	keep := hubHALabeledConfigMap("keep-cm", "default")
	stale := hubHALabeledConfigMap("stale-cm", "default")
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(keep, stale).Build()

	var deleteCalls atomicInt
	cl := interceptor.NewClient(base, interceptor.Funcs{
		Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
			if obj.GetName() == "stale-cm" {
				if deleteCalls.add(1) == 1 {
					return fmt.Errorf("transient delete failure")
				}
			}
			return c.Delete(ctx, obj, opts...)
		},
	})
	syncer := NewHubHAStandbySyncer(cl)

	syncMeta := func(metas []generic.ObjectMetadata) error {
		bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
		bundle.ResyncMetadata = metas
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubHAResourcesMsgKey)
		evt.SetSource("hub1")
		if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
			return err
		}
		return syncer.Sync(context.Background(), &evt)
	}

	if err := syncMeta([]generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", InventoryBegin: true},
		{Namespace: "default", Name: "keep-cm", Group: "", Version: "v1", Kind: "ConfigMap"},
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	}); err == nil {
		t.Fatal("expected cleanup failure on first Complete")
	}

	ctx := context.Background()
	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	if err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj); err != nil {
		t.Fatalf("stale-cm should remain after failed cleanup: %v", err)
	}

	// Retry Complete only — session must still hold keep-cm so cleanup does not delete it.
	if err := syncMeta([]generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	}); err != nil {
		t.Fatalf("retry Complete error = %v", err)
	}

	keepObj := &unstructured.Unstructured{}
	keepObj.SetAPIVersion("v1")
	keepObj.SetKind("ConfigMap")
	if err := cl.Get(ctx, types.NamespacedName{Name: "keep-cm", Namespace: "default"}, keepObj); err != nil {
		t.Fatalf("expected keep-cm to remain after retry: %v", err)
	}
	err := cl.Get(ctx, types.NamespacedName{Name: "stale-cm", Namespace: "default"}, staleObj)
	if err == nil {
		t.Fatal("expected stale-cm to be deleted after successful retry")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for stale-cm, got: %v", err)
	}
}

func TestHubHAStandbySyncer_Sync_ResyncMetadata_CompleteWithoutBegin_Ignored(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	stale := hubHALabeledConfigMap("stale-cm", "default")
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stale).Build()
	syncer := NewHubHAStandbySyncer(cl)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.ResyncMetadata = []generic.ObjectMetadata{
		{Group: "", Version: "v1", Kind: "ConfigMap", Complete: true},
	}
	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	if err := syncer.Sync(context.Background(), &evt); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion("v1")
	staleObj.SetKind("ConfigMap")
	err := cl.Get(context.Background(), types.NamespacedName{
		Name: "stale-cm", Namespace: "default",
	}, staleObj)
	if err != nil {
		t.Fatalf("Complete without Begin must not empty-delete inventory, got: %v", err)
	}
}

// TestHubHAStandbySyncer_Sync_ResyncMetadata_PreservesGlobalHubTopologyManagedClusters
// ensures ResyncMetadata stale cleanup on the global-hub standby does not delete
// imported hubs / local-cluster ManagedClusters that are not part of the active
// regional hub's spoke inventory (regression for Hub HA e2e BeforeAll).
func TestHubHAStandbySyncer_Sync_ResyncMetadata_PreservesGlobalHubTopologyManagedClusters(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	keepSpoke := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "spoke-keep",
			},
		},
	}
	staleSpoke := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "spoke-stale",
			},
		},
	}
	importedHub := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "hub1",
				"labels": map[string]interface{}{
					constants.GHDeployModeLabelKey: "default",
					constants.GHHubRoleLabelKey:    constants.GHHubRoleActive,
				},
			},
		},
	}
	otherHub := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": "hub2",
				"labels": map[string]interface{}{
					constants.GHDeployModeLabelKey: "default",
				},
			},
		},
	}
	localCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": managedClusterAPIVersion,
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"name": constants.LocalClusterName,
				"labels": map[string]interface{}{
					constants.LocalClusterName: "true",
				},
			},
		},
	}

	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(keepSpoke, staleSpoke, importedHub, otherHub, localCluster).
		Build()
	syncer := NewHubHAStandbySyncer(cl)

	bundle := generic.NewGenericBundle[*unstructured.Unstructured]()
	bundle.ResyncMetadata = []generic.ObjectMetadata{
		{
			Group: "cluster.open-cluster-management.io", Version: "v1",
			Kind: "ManagedCluster", InventoryBegin: true,
		},
		{
			Name: "spoke-keep", Group: "cluster.open-cluster-management.io",
			Version: "v1", Kind: "ManagedCluster",
		},
		{
			Group: "cluster.open-cluster-management.io", Version: "v1",
			Kind: "ManagedCluster", Complete: true,
		},
	}
	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubHAResourcesMsgKey)
	evt.SetSource("hub1")
	if err := evt.SetData(cloudevents.ApplicationJSON, bundle); err != nil {
		t.Fatalf("SetData() error = %v", err)
	}
	if err := syncer.Sync(context.Background(), &evt); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	ctx := context.Background()
	for _, name := range []string{"spoke-keep", "hub1", "hub2", constants.LocalClusterName} {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(managedClusterAPIVersion)
		obj.SetKind("ManagedCluster")
		if err := cl.Get(ctx, types.NamespacedName{Name: name}, obj); err != nil {
			t.Fatalf("expected ManagedCluster %s to remain: %v", name, err)
		}
	}

	staleObj := &unstructured.Unstructured{}
	staleObj.SetAPIVersion(managedClusterAPIVersion)
	staleObj.SetKind("ManagedCluster")
	err := cl.Get(ctx, types.NamespacedName{Name: "spoke-stale"}, staleObj)
	if err == nil {
		t.Fatal("expected spoke-stale ManagedCluster to be deleted")
	}
	if !errors.IsNotFound(err) {
		t.Fatalf("expected NotFound for spoke-stale, got: %v", err)
	}
}

// atomicInt is a tiny counter for interceptor tests without importing sync/atomic in assertions.
type atomicInt struct{ v int }

func (a *atomicInt) add(n int) int {
	a.v += n
	return a.v
}
