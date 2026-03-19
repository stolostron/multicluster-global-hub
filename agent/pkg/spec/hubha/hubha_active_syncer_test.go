// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
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

	if syncer == nil {
		t.Error("HubHAActiveSyncer creation failed")
	}

	if syncer.activeHubName != "hub1" {
		t.Errorf("activeHubName = %s, want hub1", syncer.activeHubName)
	}

	if syncer.standbyHubName != "hub2" {
		t.Errorf("standbyHubName = %s, want hub2", syncer.standbyHubName)
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
	producer := &mockProducer{}

	// Configure agent without standby hub
	agentConfig := &configs.AgentConfig{
		LeafHubName: "hub1",
		HubRole:     constants.GHHubRoleActive,
		StandbyHub:  "", // No standby hub
	}
	configs.SetAgentConfig(agentConfig)

	// This should not start the syncer
	err := StartHubHAActiveSyncer(context.Background(), nil, producer)

	// We expect no error, but the syncer shouldn't actually start
	if err != nil {
		t.Errorf("StartHubHAActiveSyncer() error = %v, expected nil", err)
	}
}

func TestStartHubHAActiveSyncer_NotActiveRole(t *testing.T) {
	producer := &mockProducer{}

	// Configure agent as standby hub
	agentConfig := &configs.AgentConfig{
		LeafHubName: "hub1",
		HubRole:     constants.GHHubRoleStandby,
		StandbyHub:  "hub2",
	}
	configs.SetAgentConfig(agentConfig)

	// This should not start the syncer
	err := StartHubHAActiveSyncer(context.Background(), nil, producer)

	if err != nil {
		t.Errorf("StartHubHAActiveSyncer() error = %v, expected nil", err)
	}
}
