// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubmanagement

import (
	"context"
	"encoding/json"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/hubha"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	localClusterName     = "local-cluster"
	testHubName          = "test-hub"
	msgSkipDBTest        = "Skipping database integration test in short mode"
	msgDBNotAvailable    = "Database not available"
	errCreateTestDataFmt = "Failed to create test data: %v"
)

func TestFindStandbyHub(t *testing.T) {
	tests := []struct {
		name          string
		clusters      []client.Object
		expectedHub   string
		expectedError bool
	}{
		{
			name: "standby hub exists",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub2",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expectedHub:   "hub2",
			expectedError: false,
		},
		{
			name: "no standby hub - returns local-cluster",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
			},
			expectedHub:   localClusterName,
			expectedError: false,
		},
		{
			name: "multiple standby hubs - returns first one",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub2",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expectedHub:   "hub1", // Returns first in list
			expectedError: false,
		},
		{
			name:          "no clusters",
			clusters:      []client.Object{},
			expectedHub:   localClusterName,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(configs.GetRuntimeScheme()).
				WithObjects(tt.clusters...).
				Build()

			hm := &HubManagement{
				client: fakeClient,
			}

			hub, err := hm.findStandbyHub(context.Background())
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedHub, hub)
			}
		})
	}
}

func TestSendHubStatusUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip(msgSkipDBTest)
	}

	db := database.GetGorm()
	if db == nil {
		t.Skip(msgDBNotAvailable)
	}

	tests := []struct {
		name           string
		hubName        string
		status         string
		setupClusters  func() []models.ManagedCluster
		setupStandby   func() []client.Object
		expectSendCall bool
	}{
		{
			name:    "send inactive status with clusters",
			hubName: "hub1",
			status:  constants.HubStatusInactive,
			setupClusters: func() []models.ManagedCluster {
				return []models.ManagedCluster{
					{
						ClusterID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
						LeafHubName: "hub1",
						Payload:     createManagedClusterJSON("cluster1"),
						Error:       database.ErrorNone,
					},
					{
						ClusterID:   "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
						LeafHubName: "hub1",
						Payload:     createManagedClusterJSON("cluster2"),
						Error:       database.ErrorNone,
					},
				}
			},
			setupStandby: func() []client.Object {
				return []client.Object{
					&clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "standby-hub",
							Labels: map[string]string{
								constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
							},
						},
					},
				}
			},
			expectSendCall: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup database
			clusters := tt.setupClusters()
			defer func() {
				db.Where("leaf_hub_name = ?", tt.hubName).Delete(&models.ManagedCluster{})
			}()
			err := db.Create(&clusters).Error
			if err != nil {
				t.Skipf(errCreateTestDataFmt, err)
			}

			// Setup k8s client with standby hub
			fakeClient := fake.NewClientBuilder().
				WithScheme(configs.GetRuntimeScheme()).
				WithObjects(tt.setupStandby()...).
				Build()

			mockProducer := &mockProducer{}
			hm := &HubManagement{
				client:   fakeClient,
				producer: mockProducer,
			}

			err = hm.sendHubStatusUpdate(context.Background(), tt.hubName, tt.status)
			assert.NoError(t, err)

			if tt.expectSendCall {
				assert.True(t, mockProducer.sendCalled, "SendEvent should have been called")
				assert.NotNil(t, mockProducer.lastEvent, "Event should not be nil")

				// Verify event type
				assert.Equal(t, constants.HubStatusUpdateMsgKey, mockProducer.lastEvent.Type())

				// Verify payload
				var update hubha.HubStatusUpdate
				err := mockProducer.lastEvent.DataAs(&update)
				assert.NoError(t, err)
				assert.Equal(t, tt.hubName, update.HubName)
				assert.Equal(t, tt.status, update.Status)
				assert.NotEmpty(t, update.ManagedClusters)
			}
		})
	}
}

func TestSendHubStatusUpdate_NoStandbyHub(t *testing.T) {
	if testing.Short() {
		t.Skip(msgSkipDBTest)
	}

	db := database.GetGorm()
	if db == nil {
		t.Skip(msgDBNotAvailable)
	}

	// Setup: No standby hub in cluster
	mockProducer := &mockProducer{}
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	hm := &HubManagement{
		client:   fakeClient,
		producer: mockProducer,
	}

	// This should fallback to local-cluster
	standbyHub, err := hm.findStandbyHub(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, localClusterName, standbyHub)

	// Setup test cluster in database
	testCluster := models.ManagedCluster{
		ClusterID:   "cccccccc-cccc-cccc-cccc-cccccccccccc",
		LeafHubName: "hub1",
		Payload:     createManagedClusterJSON("cluster1"),
		Error:       database.ErrorNone,
	}
	defer func() {
		db.Where("cluster_id = ?", testCluster.ClusterID).Delete(&models.ManagedCluster{})
	}()
	err = db.Create(&testCluster).Error
	if err != nil {
		t.Skipf(errCreateTestDataFmt, err)
	}

	// Send should work with local-cluster as fallback
	err = hm.sendHubStatusUpdate(context.Background(), "hub1", constants.HubStatusInactive)
	assert.NoError(t, err)
	assert.True(t, mockProducer.sendCalled)
}

func TestGetManagedClusterNames_Integration(t *testing.T) {
	// Skip if database is not available
	if testing.Short() {
		t.Skip(msgSkipDBTest)
	}

	// This test requires actual database connection
	// It's more of an integration test, but included here for coverage
	db := database.GetGorm()
	if db == nil {
		t.Skip(msgDBNotAvailable)
	}

	hm := &HubManagement{}

	// Insert test data
	testClusters := []models.ManagedCluster{
		{
			ClusterID:   "11111111-1111-1111-1111-111111111111",
			LeafHubName: testHubName,
			Payload: []byte(`{
				"metadata": {
					"name": "cluster1"
				}
			}`),
			Error: database.ErrorNone,
		},
		{
			ClusterID:   "22222222-2222-2222-2222-222222222222",
			LeafHubName: testHubName,
			Payload: []byte(`{
				"metadata": {
					"name": "cluster2"
				}
			}`),
			Error: database.ErrorNone,
		},
		{
			ClusterID:   "33333333-3333-3333-3333-333333333333",
			LeafHubName: "other-hub",
			Payload: []byte(`{
				"metadata": {
					"name": "cluster3"
				}
			}`),
			Error: database.ErrorNone,
		},
	}

	// Clean up first
	defer func() {
		db.Where("leaf_hub_name IN ?", []string{testHubName, "other-hub"}).Delete(&models.ManagedCluster{})
	}()

	// Insert test clusters
	err := db.Create(&testClusters).Error
	if err != nil {
		t.Skipf(errCreateTestDataFmt, err)
	}

	// Test getting cluster names for testHubName
	clusterNames, err := hm.getManagedClusterNames(testHubName)
	assert.NoError(t, err)
	assert.Len(t, clusterNames, 2)
	assert.Contains(t, clusterNames, "cluster1")
	assert.Contains(t, clusterNames, "cluster2")

	// Test getting cluster names for non-existent hub
	clusterNames, err = hm.getManagedClusterNames("nonexistent-hub")
	assert.NoError(t, err)
	assert.Empty(t, clusterNames)
}

func TestGetManagedClusterNames_EmptyResult(t *testing.T) {
	if testing.Short() {
		t.Skip(msgSkipDBTest)
	}

	db := database.GetGorm()
	if db == nil {
		t.Skip(msgDBNotAvailable)
	}

	hm := &HubManagement{}

	// Test with hub that has no clusters
	clusterNames, err := hm.getManagedClusterNames("empty-hub")
	assert.NoError(t, err)
	assert.Empty(t, clusterNames)
}

func TestSendHubStatusUpdate_ProducerError(t *testing.T) {
	if testing.Short() {
		t.Skip(msgSkipDBTest)
	}

	db := database.GetGorm()
	if db == nil {
		t.Skip(msgDBNotAvailable)
	}

	// Setup test cluster in database
	testCluster := models.ManagedCluster{
		ClusterID:   "dddddddd-dddd-dddd-dddd-dddddddddddd",
		LeafHubName: "hub1",
		Payload:     createManagedClusterJSON("cluster1"),
		Error:       database.ErrorNone,
	}
	defer func() {
		db.Where("cluster_id = ?", testCluster.ClusterID).Delete(&models.ManagedCluster{})
	}()
	err := db.Create(&testCluster).Error
	if err != nil {
		t.Skipf(errCreateTestDataFmt, err)
	}

	// Setup k8s client with standby hub
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		WithObjects(&clusterv1.ManagedCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "standby-hub",
				Labels: map[string]string{
					constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
				},
			},
		}).
		Build()

	// Producer that returns error
	mockProducer := &mockProducer{
		sendError: assert.AnError,
	}
	hm := &HubManagement{
		client:   fakeClient,
		producer: mockProducer,
	}

	// Should return error from producer
	err = hm.sendHubStatusUpdate(context.Background(), "hub1", constants.HubStatusInactive)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send hub status update")
}

func TestSendHubStatusUpdate_NoStandbyHubAvailable(t *testing.T) {
	// Setup: Standby hub returns empty string (no standby available)
	fakeClient := fake.NewClientBuilder().
		WithScheme(configs.GetRuntimeScheme()).
		Build()

	mockProducer := &mockProducer{}
	hm := &HubManagement{
		client:   fakeClient,
		producer: mockProducer,
	}

	// findStandbyHub returns local-cluster when no standby found
	// sendHubStatusUpdate should still work
	standbyHub, err := hm.findStandbyHub(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, localClusterName, standbyHub)
}

// mockProducer implements transport.Producer for testing
type mockProducer struct {
	sendCalled bool
	lastEvent  *cloudevents.Event
	sendError  error
}

func (m *mockProducer) SendEvent(ctx context.Context, evt cloudevents.Event) error {
	m.sendCalled = true
	m.lastEvent = &evt
	return m.sendError
}

func (m *mockProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

// Helper function to create test ManagedCluster JSON
func createManagedClusterJSON(name string) []byte {
	cluster := map[string]any{
		"metadata": map[string]any{
			"name": name,
		},
	}
	data, _ := json.Marshal(cluster)
	return data
}
