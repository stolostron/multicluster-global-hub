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

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
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

func TestHubHAActiveSyncer_Creation(t *testing.T) {
	scheme := runtime.NewScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	producer := &mockProducer{}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	syncer := &HubHAActiveSyncer{
		client:          client,
		producer:        producer,
		transportConfig: transportConfig,
		activeHubName:   "hub1",
		standbyHubName:  "hub2",
	}

	// Discover resources
	err := syncer.discoverResources()
	if err != nil {
		t.Errorf("discoverResources() error = %v", err)
	}

	if len(syncer.resourcesToSync) == 0 {
		t.Error("resourcesToSync is empty")
	}
}

func TestSyncResources(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create test objects
	policy := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "policy.open-cluster-management.io/v1",
			"kind":       "Policy",
			"metadata": map[string]interface{}{
				"name":      "test-policy",
				"namespace": "default",
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(policy).
		Build()

	producer := &mockProducer{events: []cloudevents.Event{}}
	transportConfig := &transport.TransportInternalConfig{
		KafkaCredential: &transport.KafkaConfig{
			SpecTopic: "spec-topic",
		},
	}

	syncer := &HubHAActiveSyncer{
		client:          client,
		producer:        producer,
		transportConfig: transportConfig,
		activeHubName:   "hub1",
		standbyHubName:  "hub2",
	}

	// Override resourcesToSync to only test Policy to avoid CRD not found errors
	syncer.resourcesToSync = []schema.GroupVersionKind{
		{Group: "policy.open-cluster-management.io", Version: "v1", Kind: "Policy"},
	}

	ctx := context.Background()
	err := syncer.syncResources(ctx)
	if err != nil {
		t.Errorf("syncResources() error = %v", err)
	}

	if len(producer.events) != 1 {
		t.Errorf("Expected 1 event to be sent, got %d", len(producer.events))
	}

	if len(producer.events) > 0 {
		evt := producer.events[0]
		if evt.Type() != constants.HubHAResourcesMsgKey {
			t.Errorf("Event type = %s, want %s", evt.Type(), constants.HubHAResourcesMsgKey)
		}
		if evt.Source() != "hub1" {
			t.Errorf("Event source = %s, want hub1", evt.Source())
		}
	}
}

func TestStartHubHAActiveSyncer_NoStandbyHub(t *testing.T) {
	// Capture and restore original agent config to avoid test pollution
	originalConfig := configs.GetAgentConfig()
	t.Cleanup(func() {
		configs.SetAgentConfig(originalConfig)
	})

	producer := &mockProducer{}

	// Configure agent without standby hub
	agentConfig := &configs.AgentConfig{
		LeafHubName: "hub1",
	}
	agentConfig.SetHubRole(constants.GHHubRoleActive)
	agentConfig.SetStandbyHub("") // No standby hub
	configs.SetAgentConfig(agentConfig)

	// This should not start the syncer
	err := StartHubHAActiveSyncer(context.Background(), nil, producer)
	// We expect no error, but the syncer shouldn't actually start
	if err != nil {
		t.Errorf("StartHubHAActiveSyncer() error = %v, expected nil", err)
	}
}

func TestStartHubHAActiveSyncer_NotActiveRole(t *testing.T) {
	// Capture and restore original agent config to avoid test pollution
	originalConfig := configs.GetAgentConfig()
	t.Cleanup(func() {
		configs.SetAgentConfig(originalConfig)
	})

	producer := &mockProducer{}

	// Configure agent as standby hub
	agentConfig := &configs.AgentConfig{
		LeafHubName: "hub1",
	}
	agentConfig.SetHubRole(constants.GHHubRoleStandby)
	agentConfig.SetStandbyHub("hub2")
	configs.SetAgentConfig(agentConfig)

	// This should not start the syncer
	err := StartHubHAActiveSyncer(context.Background(), nil, producer)
	if err != nil {
		t.Errorf("StartHubHAActiveSyncer() error = %v, expected nil", err)
	}
}

func TestHubHAActiveSyncer_ConfigMapPersistence(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := &HubHAActiveSyncer{
		client:    client,
		namespace: "test-namespace",
		previousResources: map[string]generic.ObjectMetadata{
			"test-key-1": {
				Namespace: "default",
				Name:      "test-resource-1",
				Group:     "",
				Version:   "v1",
				Kind:      "ConfigMap",
			},
			"test-key-2": {
				Namespace: "default",
				Name:      "test-resource-2",
				Group:     "policy.open-cluster-management.io",
				Version:   "v1",
				Kind:      "Policy",
			},
		},
	}

	ctx := context.Background()

	// Test saving to ConfigMap
	err := syncer.savePreviousResourcesToConfigMap(ctx)
	if err != nil {
		t.Errorf("savePreviousResourcesToConfigMap() error = %v", err)
	}

	// Verify ConfigMap was created
	cm := &corev1.ConfigMap{}
	err = client.Get(ctx, types.NamespacedName{
		Name:      hubHAStateConfigMapName,
		Namespace: "test-namespace",
	}, cm)
	if err != nil {
		t.Errorf("Failed to get ConfigMap: %v", err)
	}

	if cm.Data["state"] == "" {
		t.Error("ConfigMap state data is empty")
	}

	// Test loading from ConfigMap
	syncer2 := &HubHAActiveSyncer{
		client:            client,
		namespace:         "test-namespace",
		previousResources: make(map[string]generic.ObjectMetadata),
	}

	err = syncer2.loadPreviousResourcesFromConfigMap(ctx)
	if err != nil {
		t.Errorf("loadPreviousResourcesFromConfigMap() error = %v", err)
	}

	// Verify loaded state matches saved state
	if len(syncer2.previousResources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(syncer2.previousResources))
	}

	res1, exists := syncer2.previousResources["test-key-1"]
	if !exists {
		t.Error("test-key-1 not found in loaded resources")
	} else {
		if res1.Name != "test-resource-1" || res1.Kind != "ConfigMap" {
			t.Errorf("Loaded resource mismatch: %+v", res1)
		}
	}

	res2, exists := syncer2.previousResources["test-key-2"]
	if !exists {
		t.Error("test-key-2 not found in loaded resources")
	} else {
		if res2.Name != "test-resource-2" || res2.Kind != "Policy" {
			t.Errorf("Loaded resource mismatch: %+v", res2)
		}
	}
}

func TestHubHAActiveSyncer_LoadNonExistentConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	syncer := &HubHAActiveSyncer{
		client:            client,
		namespace:         "test-namespace",
		previousResources: make(map[string]generic.ObjectMetadata),
	}

	ctx := context.Background()

	// Loading from non-existent ConfigMap should not error
	err := syncer.loadPreviousResourcesFromConfigMap(ctx)
	if err != nil {
		t.Errorf("loadPreviousResourcesFromConfigMap() error = %v, expected nil for non-existent ConfigMap", err)
	}

	// previousResources should remain empty
	if len(syncer.previousResources) != 0 {
		t.Errorf("Expected 0 resources, got %d", len(syncer.previousResources))
	}
}
