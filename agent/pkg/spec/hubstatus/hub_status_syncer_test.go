// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubstatus

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestHubStatusSyncer_Sync_ActiveStatus(t *testing.T) {
	// Create fake client with existing ManagedCluster
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = "cluster1"
	managedCluster.Spec.HubAcceptsClient = true // Initially set to true

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(managedCluster).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message - active status
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusActive,
		ManagedClusters: []string{"cluster1"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should update hubAcceptsClient to false (active hub is healthy)
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)

	// Verify hubAcceptsClient was set to false
	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	assert.NoError(t, err)
	assert.False(t, result.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_InactiveStatus(t *testing.T) {
	// Create fake client with existing ManagedCluster
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = "cluster1"
	managedCluster.Spec.HubAcceptsClient = false // Initially set to false

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(managedCluster).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message - inactive status
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusInactive,
		ManagedClusters: []string{"cluster1"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should update hubAcceptsClient to true (active hub is down - failover)
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)

	// Verify hubAcceptsClient was set to true
	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	assert.NoError(t, err)
	assert.True(t, result.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_MultipleClusters(t *testing.T) {
	// Create fake client with multiple ManagedClusters
	cluster1 := &clusterv1.ManagedCluster{}
	cluster1.Name = "cluster1"
	cluster1.Spec.HubAcceptsClient = false

	cluster2 := &clusterv1.ManagedCluster{}
	cluster2.Name = "cluster2"
	cluster2.Spec.HubAcceptsClient = false

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(cluster1, cluster2).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message with multiple clusters
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusInactive,
		ManagedClusters: []string{"cluster1", "cluster2"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should update both clusters
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)

	// Verify both clusters were updated
	result1 := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result1)
	assert.NoError(t, err)
	assert.True(t, result1.Spec.HubAcceptsClient)

	result2 := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster2"}, result2)
	assert.NoError(t, err)
	assert.True(t, result2.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_ClusterNotFound(t *testing.T) {
	// Create fake client without the cluster
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusInactive,
		ManagedClusters: []string{"nonexistent-cluster"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should not error even if cluster doesn't exist (it's logged and skipped)
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)
}

func TestHubStatusSyncer_Sync_NoUpdateNeeded(t *testing.T) {
	// Create fake client with ManagedCluster already at desired value
	managedCluster := &clusterv1.ManagedCluster{}
	managedCluster.Name = "cluster1"
	managedCluster.Spec.HubAcceptsClient = false

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(managedCluster).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message with active status (should set to false, already false)
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusActive,
		ManagedClusters: []string{"cluster1"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should succeed without update
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)

	// Verify value unchanged
	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	assert.NoError(t, err)
	assert.False(t, result.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_WrongEventType(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create event with wrong type
	evt := cloudevents.NewEvent()
	evt.SetType("WrongType")
	evt.SetSource("manager")

	// Sync should return nil (skip processing)
	err := syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)
}

func TestHubStatusSyncer_Sync_PartialFailure(t *testing.T) {
	// Create fake client with only one cluster (cluster1 exists, cluster2 doesn't)
	managedCluster1 := &clusterv1.ManagedCluster{}
	managedCluster1.Name = "cluster1"
	managedCluster1.Spec.HubAcceptsClient = false

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(managedCluster1).
		Build()

	syncer := &HubStatusSyncer{
		client: fakeClient,
	}

	// Create hub status update message with two clusters (one exists, one doesn't)
	update := hubha.HubStatusUpdate{
		HubName:         "hub1",
		Status:          constants.HubStatusInactive,
		ManagedClusters: []string{"cluster1", "cluster2"},
	}
	payload, _ := json.Marshal(update)

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	err := evt.SetData(cloudevents.ApplicationJSON, payload)
	assert.NoError(t, err)

	// Sync should succeed - NotFound errors are ignored
	err = syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err)

	// Verify cluster1 was updated
	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	assert.NoError(t, err)
	assert.True(t, result.Spec.HubAcceptsClient)
}
