// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubstatus

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	// Sync should return error so the message is retried (cache sync delay)
	err = syncer.Sync(context.TODO(), &evt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-cluster")
	assert.Contains(t, err.Error(), "not found")
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

	// Sync should return error for missing cluster2, triggering message retry
	err = syncer.Sync(context.TODO(), &evt)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cluster2")

	// cluster1 should still have been updated before the aggregate error is returned
	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	assert.NoError(t, err)
	assert.True(t, result.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_FailoverThenRecovery(t *testing.T) {
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

	syncer := &HubStatusSyncer{client: fakeClient}
	clusters := []string{"cluster1", "cluster2"}

	// Step 1: Failover — active hub goes inactive
	failoverPayload, _ := json.Marshal(hubha.HubStatusUpdate{
		HubName: "hub1", Status: constants.HubStatusInactive, ManagedClusters: clusters,
	})
	failoverEvt := cloudevents.NewEvent()
	failoverEvt.SetType(constants.HubStatusUpdateMsgKey)
	failoverEvt.SetSource("manager")
	_ = failoverEvt.SetData(cloudevents.ApplicationJSON, failoverPayload)

	err := syncer.Sync(context.TODO(), &failoverEvt)
	require.NoError(t, err)

	for _, name := range clusters {
		mc := &clusterv1.ManagedCluster{}
		require.NoError(t, fakeClient.Get(context.TODO(), client.ObjectKey{Name: name}, mc))
		assert.True(t, mc.Spec.HubAcceptsClient, "cluster %s should accept clients after failover", name)
	}

	// Step 2: Recovery — active hub comes back
	recoveryPayload, _ := json.Marshal(hubha.HubStatusUpdate{
		HubName: "hub1", Status: constants.HubStatusActive, ManagedClusters: clusters,
	})
	recoveryEvt := cloudevents.NewEvent()
	recoveryEvt.SetType(constants.HubStatusUpdateMsgKey)
	recoveryEvt.SetSource("manager")
	_ = recoveryEvt.SetData(cloudevents.ApplicationJSON, recoveryPayload)

	err = syncer.Sync(context.TODO(), &recoveryEvt)
	require.NoError(t, err)

	for _, name := range clusters {
		mc := &clusterv1.ManagedCluster{}
		require.NoError(t, fakeClient.Get(context.TODO(), client.ObjectKey{Name: name}, mc))
		assert.False(t, mc.Spec.HubAcceptsClient, "cluster %s should not accept clients after recovery", name)
	}
}

func TestHubStatusSyncer_Sync_IdempotentFailover(t *testing.T) {
	cluster := &clusterv1.ManagedCluster{}
	cluster.Name = "cluster1"
	cluster.Spec.HubAcceptsClient = false

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(cluster).
		Build()

	syncer := &HubStatusSyncer{client: fakeClient}

	payload, _ := json.Marshal(hubha.HubStatusUpdate{
		HubName: "hub1", Status: constants.HubStatusInactive, ManagedClusters: []string{"cluster1"},
	})

	// Send the same failover event twice
	for i := 0; i < 2; i++ {
		evt := cloudevents.NewEvent()
		evt.SetType(constants.HubStatusUpdateMsgKey)
		evt.SetSource("manager")
		_ = evt.SetData(cloudevents.ApplicationJSON, payload)

		err := syncer.Sync(context.TODO(), &evt)
		require.NoError(t, err, "iteration %d", i)
	}

	mc := &clusterv1.ManagedCluster{}
	require.NoError(t, fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, mc))
	assert.True(t, mc.Spec.HubAcceptsClient)
}

func TestHubStatusSyncer_Sync_EmptyClusterList(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	syncer := &HubStatusSyncer{client: fakeClient}

	payload, _ := json.Marshal(hubha.HubStatusUpdate{
		HubName: "hub1", Status: constants.HubStatusInactive, ManagedClusters: []string{},
	})

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	_ = evt.SetData(cloudevents.ApplicationJSON, payload)

	err := syncer.Sync(context.TODO(), &evt)
	assert.NoError(t, err, "empty cluster list should not error")
}

func TestHubStatusSyncer_Sync_MalformedPayload(t *testing.T) {
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	syncer := &HubStatusSyncer{client: fakeClient}

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	_ = evt.SetData(cloudevents.ApplicationJSON, []byte("not valid json"))

	err := syncer.Sync(context.TODO(), &evt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse hub status update payload")
}

func TestHubStatusSyncer_Sync_UnknownStatusValue(t *testing.T) {
	cluster := &clusterv1.ManagedCluster{}
	cluster.Name = "cluster1"
	cluster.Spec.HubAcceptsClient = true

	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(cluster).
		Build()

	syncer := &HubStatusSyncer{client: fakeClient}

	payload, _ := json.Marshal(hubha.HubStatusUpdate{
		HubName: "hub1", Status: "degraded", ManagedClusters: []string{"cluster1"},
	})

	evt := cloudevents.NewEvent()
	evt.SetType(constants.HubStatusUpdateMsgKey)
	evt.SetSource("manager")
	_ = evt.SetData(cloudevents.ApplicationJSON, payload)

	err := syncer.Sync(context.TODO(), &evt)
	require.NoError(t, err)

	result := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.TODO(), client.ObjectKey{Name: "cluster1"}, result)
	require.NoError(t, err)
	assert.False(t, result.Spec.HubAcceptsClient,
		"unknown status should be treated as active (hubAcceptsClient=false)")
}
