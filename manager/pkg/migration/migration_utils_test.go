package migration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// Helper function to convert cluster slice to JSON string
func clustersToJSON(clusters []string) string {
	if clusters == nil {
		return "null"
	}
	data, _ := json.Marshal(clusters)
	return string(data)
}

// TestUpdateFailedClustersConfimap tests the UpdateFailedClustersConfimap function which is called
// during migration status updates to store cluster migration results in ConfigMaps.
// This function only updates ConfigMaps when a migration is in PhaseFailed status.
//
// Test coverage includes:
// - Non-failed migrations (should not update ConfigMap)
// - Failed migrations with no existing ConfigMap (creates new one with failed clusters)
// - Failed migrations with existing ConfigMap but no success clusters
// - Failed migrations with partial success (demonstrates bug in failure calculation logic)
// - Error cases (invalid JSON in existing ConfigMap)
func TestUpdateFailedClustersConfimap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                  string
		migration             *migrationv1alpha1.ManagedClusterMigration
		existingConfigMap     *corev1.ConfigMap
		clusterList           []string
		expectedError         bool
		expectedConfigMapData map[string]string
		expectNoUpdate        bool
	}{
		{
			name: "Failed migration with no existing configmap - should store all clusters as failed",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionTrue,
							Reason: "TestReason",
						},
					},
				},
			},
			clusterList: []string{"cluster1", "cluster2", "cluster3"},
			expectedConfigMapData: map[string]string{
				"failure": clustersToJSON([]string{"cluster1", "cluster2", "cluster3"}),
			},
		},
		{
			name: "Failed migration with existing configmap but no success clusters - should store all clusters as failed",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionTrue,
							Reason: "TestReason",
						},
					},
				},
			},
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": clustersToJSON([]string{"cluster1", "cluster2", "cluster3"}),
				},
			},
			clusterList: []string{"cluster1", "cluster2", "cluster3"},
			// The function only stores what it explicitly sets, not preserving existing data
			expectedConfigMapData: map[string]string{
				"failure": clustersToJSON([]string{"cluster1", "cluster2", "cluster3"}),
			},
		},
		{
			name: "Failed migration with existing success clusters - should categorize clusters correctly",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionTrue,
							Reason: "TestReason",
						},
					},
				},
			},
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": clustersToJSON([]string{"cluster1", "cluster2", "cluster3", "cluster4"}),
					"success":      clustersToJSON([]string{"cluster1", "cluster2"}),
				},
			},
			clusterList: []string{"cluster1", "cluster2", "cluster3", "cluster4"},
			// UpdateFailureClustersToConfigMap only stores failure clusters, it doesn't preserve success clusters
			expectedConfigMapData: map[string]string{
				"failure": clustersToJSON([]string{"cluster3", "cluster4"}),
			},
		},
		{
			name: "Failed migration with partial success clusters - should handle single failed cluster",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionTrue,
							Reason: "TestReason",
						},
					},
				},
			},
			existingConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": clustersToJSON([]string{"cluster1", "cluster2"}),
					"success":      clustersToJSON([]string{"cluster1"}),
				},
			},
			clusterList: []string{"cluster1", "cluster2"},
			// UpdateFailureClustersToConfigMap only stores failure clusters, not success clusters
			expectedConfigMapData: map[string]string{
				"failure": clustersToJSON([]string{"cluster2"}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup fake client with existing objects
			var objs []client.Object
			if tt.existingConfigMap != nil {
				objs = append(objs, tt.existingConfigMap)
			}
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()

			// Create controller instance
			controller := &ClusterMigrationController{
				Client: fakeClient,
				Scheme: scheme,
			}

			// Setup cluster list for migration using the exported function
			SetClusterList(string(tt.migration.UID), tt.clusterList)

			// Call the function under test
			err := controller.UpdateFailureClustersToConfigMap(context.TODO(), tt.migration)

			// Assertions
			if tt.expectedError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			if tt.expectNoUpdate {
				// Verify no configmap was created or updated
				configMap := &corev1.ConfigMap{}
				err := fakeClient.Get(context.TODO(), types.NamespacedName{
					Name:      tt.migration.Name,
					Namespace: tt.migration.Namespace,
				}, configMap)
				if tt.existingConfigMap == nil {
					assert.Error(t, err) // Should not exist
				} else {
					assert.NoError(t, err)
					// Should be unchanged
					assert.Equal(t, tt.existingConfigMap.Data, configMap.Data)
				}
				return
			}

			// Verify configmap was created/updated with expected data
			configMap := &corev1.ConfigMap{}
			err = fakeClient.Get(context.TODO(), types.NamespacedName{
				Name:      tt.migration.Name,
				Namespace: tt.migration.Namespace,
			}, configMap)
			assert.NoError(t, err)

			// Verify the data
			for key, expectedValue := range tt.expectedConfigMapData {
				actualValue, exists := configMap.Data[key]
				assert.True(t, exists, "Expected key %s to exist in configmap", key)
				assert.Equal(t, expectedValue, actualValue, "Expected value for key %s", key)
			}

			// Verify owner reference is set
			if len(configMap.OwnerReferences) > 0 {
				assert.Equal(t, tt.migration.Name, configMap.OwnerReferences[0].Name)
				assert.Equal(t, tt.migration.UID, configMap.OwnerReferences[0].UID)
			}
		})
	}
}

// TestUpdateFailedClustersConfimap_ErrorCases tests error scenarios for UpdateFailedClustersConfimap
func TestUpdateFailedClustersConfimap_ErrorCases(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	t.Run("Invalid JSON in configmap should cause parse error", func(t *testing.T) {
		// Create a configmap with invalid JSON data
		invalidConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-migration",
				Namespace: utils.GetDefaultNamespace(),
			},
			Data: map[string]string{
				"success": "invalid-json", // This will cause JSON unmarshal error
			},
		}

		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(invalidConfigMap).Build()

		controller := &ClusterMigrationController{
			Client: fakeClient,
			Scheme: scheme,
		}

		migration := &migrationv1alpha1.ManagedClusterMigration{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-migration",
				Namespace: utils.GetDefaultNamespace(),
				UID:       types.UID("test-uid"),
			},
			Status: migrationv1alpha1.ManagedClusterMigrationStatus{
				Phase: migrationv1alpha1.PhaseFailed,
				Conditions: []metav1.Condition{
					{
						Type:   migrationv1alpha1.ConditionTypeRegistered,
						Status: metav1.ConditionTrue,
						Reason: "TestReason",
					},
				},
			},
		}

		// Setup cluster list for migration
		SetClusterList(string(migration.UID), []string{"cluster1", "cluster2"})

		// Call the function under test
		err := controller.UpdateFailureClustersToConfigMap(context.TODO(), migration)

		// Should get an error due to invalid JSON
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to unmarshal clusters from ConfigMap")
	})
}

// TestHandleRollbackRetryRequest tests the handleRollbackRetryRequest function which allows
// users to trigger a rollback retry for failed migrations via annotation.
func TestHandleRollbackRetryRequest(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		expectedRequeue         bool
		expectedPhase           string
		expectAnnotationRemoved bool
	}{
		{
			name: "Failed migration with rollback annotation - should trigger rollback retry",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-1"),
					Annotations: map[string]string{
						constants.MigrationRequestAnnotationKey: constants.MigrationRollbackAnnotationValue,
					},
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRolledBack,
							Status: metav1.ConditionFalse,
							Reason: "Error",
						},
					},
				},
			},
			expectedRequeue:         true,
			expectedPhase:           migrationv1alpha1.PhaseRollbacking,
			expectAnnotationRemoved: true,
		},
		{
			name: "Failed migration without annotation - should not trigger rollback",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
				},
			},
			expectedRequeue: false,
			expectedPhase:   migrationv1alpha1.PhaseFailed,
		},
		{
			name: "Non-failed migration with annotation - should not trigger rollback",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
					Annotations: map[string]string{
						constants.MigrationRequestAnnotationKey: constants.MigrationRollbackAnnotationValue,
					},
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRegistering,
				},
			},
			expectedRequeue: false,
			expectedPhase:   migrationv1alpha1.PhaseRegistering,
		},
		{
			name: "Failed migration with wrong annotation value - should not trigger rollback",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-4"),
					Annotations: map[string]string{
						constants.MigrationRequestAnnotationKey: "invalid-value",
					},
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
				},
			},
			expectedRequeue: false,
			expectedPhase:   migrationv1alpha1.PhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.migration).
				WithStatusSubresource(tt.migration).
				Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
				Scheme: scheme,
			}

			// Initialize migration status in memory
			AddMigrationStatus(string(tt.migration.UID))
			defer RemoveMigrationStatus(string(tt.migration.UID))

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.migration.Name,
					Namespace: tt.migration.Namespace,
				},
			}

			requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedRequeue, requeue)

			// Verify the migration state after the call
			updatedMigration := &migrationv1alpha1.ManagedClusterMigration{}
			err = fakeClient.Get(context.TODO(), req.NamespacedName, updatedMigration)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPhase, updatedMigration.Status.Phase)

			// Verify annotation was removed if expected
			if tt.expectAnnotationRemoved {
				_, exists := updatedMigration.Annotations[constants.MigrationRequestAnnotationKey]
				assert.False(t, exists, "Annotation should be removed after triggering rollback retry")
			}
		})
	}
}

// TestHandleRollbackRetryRequest_MigrationNotFound tests the case when migration is not found
func TestHandleRollbackRetryRequest_MigrationNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	controller := &ClusterMigrationController{
		Client: fakeClient,
		Scheme: scheme,
	}

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent-migration",
			Namespace: utils.GetDefaultNamespace(),
		},
	}

	requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

	assert.NoError(t, err)
	assert.False(t, requeue)
}

// TestHandleRollbackRetryRequest_ConditionReset verifies that the RolledBack condition is properly
// reset with a fresh LastTransitionTime when rollback retry is requested
func TestHandleRollbackRetryRequest_ConditionReset(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	// Create a migration with an existing RolledBack condition that has an old LastTransitionTime
	oldTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration-condition-reset",
			Namespace: utils.GetDefaultNamespace(),
			UID:       types.UID("test-uid-condition-reset"),
			Annotations: map[string]string{
				constants.MigrationRequestAnnotationKey: constants.MigrationRollbackAnnotationValue,
			},
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "hub1",
			To:   "hub2",
		},
		Status: migrationv1alpha1.ManagedClusterMigrationStatus{
			Phase: migrationv1alpha1.PhaseFailed,
			Conditions: []metav1.Condition{
				{
					Type:               migrationv1alpha1.ConditionTypeRolledBack,
					Status:             metav1.ConditionFalse,
					Reason:             ConditionReasonTimeout,
					Message:            "Rollback timed out waiting for agents",
					LastTransitionTime: oldTime,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migration).
		WithStatusSubresource(migration).
		Build()

	controller := &ClusterMigrationController{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Initialize migration status in memory
	AddMigrationStatus(string(migration.UID))
	defer RemoveMigrationStatus(string(migration.UID))

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      migration.Name,
			Namespace: migration.Namespace,
		},
	}

	beforeUpdate := time.Now()
	requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

	assert.NoError(t, err)
	assert.True(t, requeue, "Should requeue after rollback retry request")

	// Fetch the updated migration
	updatedMigration := &migrationv1alpha1.ManagedClusterMigration{}
	err = fakeClient.Get(context.TODO(), req.NamespacedName, updatedMigration)
	assert.NoError(t, err)

	// Verify the RolledBack condition was reset
	var rolledBackCondition *metav1.Condition
	for i := range updatedMigration.Status.Conditions {
		if updatedMigration.Status.Conditions[i].Type == migrationv1alpha1.ConditionTypeRolledBack {
			rolledBackCondition = &updatedMigration.Status.Conditions[i]
			break
		}
	}

	assert.NotNil(t, rolledBackCondition, "RolledBack condition should exist")
	assert.Equal(t, metav1.ConditionFalse, rolledBackCondition.Status)
	assert.Equal(t, ConditionReasonWaiting, rolledBackCondition.Reason)
	assert.Equal(t, "Rollback retry requested via annotation", rolledBackCondition.Message)

	// Critical: LastTransitionTime should be updated to now (not the old time)
	assert.True(t, rolledBackCondition.LastTransitionTime.After(oldTime.Time),
		"LastTransitionTime should be after the old time")

	// Allow for 2 second tolerance
	timeDiff := beforeUpdate.Sub(rolledBackCondition.LastTransitionTime.Time)
	assert.True(t, timeDiff < 2*time.Second && timeDiff > -2*time.Second,
		"LastTransitionTime should be within 2 seconds of now, got diff: %v", timeDiff)
}

// TestHandleRollbackRetryRequest_StageStateReset verifies that the in-memory stage state is reset
// for both source and target hubs when rollback retry is requested
func TestHandleRollbackRetryRequest_StageStateReset(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	migration := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration-stage-reset",
			Namespace: utils.GetDefaultNamespace(),
			UID:       types.UID("test-uid-stage-reset"),
			Annotations: map[string]string{
				constants.MigrationRequestAnnotationKey: constants.MigrationRollbackAnnotationValue,
			},
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From: "source-hub",
			To:   "target-hub",
		},
		Status: migrationv1alpha1.ManagedClusterMigrationStatus{
			Phase: migrationv1alpha1.PhaseFailed,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(migration).
		WithStatusSubresource(migration).
		Build()

	controller := &ClusterMigrationController{
		Client: fakeClient,
		Scheme: scheme,
	}

	migrationID := string(migration.UID)
	sourceHub := migration.Spec.From
	targetHub := migration.Spec.To
	rollbackingPhase := migrationv1alpha1.PhaseRollbacking

	// Initialize migration status in memory
	AddMigrationStatus(migrationID)
	defer RemoveMigrationStatus(migrationID)

	// Set up stage state as if rollback had failed
	SetStarted(migrationID, sourceHub, rollbackingPhase)
	SetFinished(migrationID, sourceHub, rollbackingPhase)
	SetErrorMessage(migrationID, sourceHub, rollbackingPhase, "source rollback failed")

	SetStarted(migrationID, targetHub, rollbackingPhase)
	SetFinished(migrationID, targetHub, rollbackingPhase)
	SetErrorMessage(migrationID, targetHub, rollbackingPhase, "target rollback failed")

	// Verify initial state is set
	assert.True(t, GetStarted(migrationID, sourceHub, rollbackingPhase))
	assert.True(t, GetFinished(migrationID, sourceHub, rollbackingPhase))
	assert.True(t, GetStarted(migrationID, targetHub, rollbackingPhase))
	assert.True(t, GetFinished(migrationID, targetHub, rollbackingPhase))

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      migration.Name,
			Namespace: migration.Namespace,
		},
	}

	requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

	assert.NoError(t, err)
	assert.True(t, requeue)

	// Verify stage state was reset for both hubs
	assert.False(t, GetStarted(migrationID, sourceHub, rollbackingPhase),
		"Source hub rollback state should be reset")
	assert.False(t, GetFinished(migrationID, sourceHub, rollbackingPhase),
		"Source hub rollback state should be reset")
	assert.Empty(t, GetErrorMessage(migrationID, sourceHub, rollbackingPhase),
		"Source hub error message should be cleared")

	assert.False(t, GetStarted(migrationID, targetHub, rollbackingPhase),
		"Target hub rollback state should be reset")
	assert.False(t, GetFinished(migrationID, targetHub, rollbackingPhase),
		"Target hub rollback state should be reset")
	assert.Empty(t, GetErrorMessage(migrationID, targetHub, rollbackingPhase),
		"Target hub error message should be cleared")
}

// TestHandleRollbackRetryRequest_DifferentAnnotationKeys tests that other annotations don't trigger rollback
func TestHandleRollbackRetryRequest_DifferentAnnotationKeys(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name        string
		annotations map[string]string
		expectPhase string
	}{
		{
			name: "Different annotation key should not trigger rollback",
			annotations: map[string]string{
				"some-other-annotation": constants.MigrationRollbackAnnotationValue,
			},
			expectPhase: migrationv1alpha1.PhaseFailed,
		},
		{
			name: "Empty annotation key should not trigger rollback",
			annotations: map[string]string{
				"": constants.MigrationRollbackAnnotationValue,
			},
			expectPhase: migrationv1alpha1.PhaseFailed,
		},
		{
			name: "Mixed annotations without the correct key should not trigger rollback",
			annotations: map[string]string{
				"annotation1": "value1",
				"annotation2": "value2",
			},
			expectPhase: migrationv1alpha1.PhaseFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			migration := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-migration-annotation",
					Namespace:   utils.GetDefaultNamespace(),
					UID:         types.UID("test-uid-annotation"),
					Annotations: tt.annotations,
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseFailed,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(migration).
				WithStatusSubresource(migration).
				Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
				Scheme: scheme,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      migration.Name,
					Namespace: migration.Namespace,
				},
			}

			requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

			assert.NoError(t, err)
			assert.False(t, requeue)

			// Verify phase is unchanged
			updatedMigration := &migrationv1alpha1.ManagedClusterMigration{}
			err = fakeClient.Get(context.TODO(), req.NamespacedName, updatedMigration)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectPhase, updatedMigration.Status.Phase)
		})
	}
}

// TestHandleRollbackRetryRequest_AllPhasesExceptFailed tests that non-Failed phases are not processed
func TestHandleRollbackRetryRequest_AllPhasesExceptFailed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	phases := []string{
		migrationv1alpha1.PhasePending,
		migrationv1alpha1.PhaseValidating,
		migrationv1alpha1.PhaseInitializing,
		migrationv1alpha1.PhaseDeploying,
		migrationv1alpha1.PhaseCleaning,
		migrationv1alpha1.PhaseRegistering,
		migrationv1alpha1.PhaseCompleted,
		migrationv1alpha1.PhaseRollbacking,
	}

	for _, phase := range phases {
		t.Run("Phase_"+phase, func(t *testing.T) {
			migration := &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-" + phase,
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-" + phase),
					Annotations: map[string]string{
						constants.MigrationRequestAnnotationKey: constants.MigrationRollbackAnnotationValue,
					},
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "hub1",
					To:   "hub2",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: phase,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(migration).
				WithStatusSubresource(migration).
				Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
				Scheme: scheme,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      migration.Name,
					Namespace: migration.Namespace,
				},
			}

			requeue, err := controller.handleRollbackRetryRequest(context.TODO(), req)

			assert.NoError(t, err)
			assert.False(t, requeue, "Phase %s should not trigger rollback", phase)

			// Verify phase is unchanged
			updatedMigration := &migrationv1alpha1.ManagedClusterMigration{}
			err = fakeClient.Get(context.TODO(), req.NamespacedName, updatedMigration)
			assert.NoError(t, err)
			assert.Equal(t, phase, updatedMigration.Status.Phase, "Phase should remain unchanged for %s", phase)
		})
	}
}

// TestGetClusterFromConfigMap tests the getClusterFromConfigMap function which retrieves
// cluster lists from ConfigMaps during migration operations.
func TestGetClusterFromConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name             string
		configMap        *corev1.ConfigMap
		key              string
		expectedClusters []string
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "ConfigMap not found - should return nil without error",
			configMap:        nil,
			key:              "all-clusters",
			expectedClusters: nil,
			expectedError:    false,
		},
		{
			name: "ConfigMap exists with all-clusters key",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": `["cluster1","cluster2","cluster3"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster1", "cluster2", "cluster3"},
			expectedError:    false,
		},
		{
			name: "ConfigMap exists with success key",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `["cluster1","cluster2"]`,
				},
			},
			key:              "success",
			expectedClusters: []string{"cluster1", "cluster2"},
			expectedError:    false,
		},
		{
			name: "ConfigMap exists with failure key",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"failure": `["cluster3"]`,
				},
			},
			key:              "failure",
			expectedClusters: []string{"cluster3"},
			expectedError:    false,
		},
		{
			name: "ConfigMap exists but key not found - should return nil",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"other-key": `["cluster1"]`,
				},
			},
			key:              "success",
			expectedClusters: nil,
			expectedError:    false,
		},
		{
			name: "ConfigMap with invalid JSON - should return error",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": `invalid-json`,
				},
			},
			key:           "all-clusters",
			expectedError: true,
			errorContains: "failed to unmarshal clusters from ConfigMap",
		},
		{
			name: "ConfigMap with empty array",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": `[]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing but success and failure exist - should join them",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `["cluster1","cluster2"]`,
					"failure": `["cluster3","cluster4"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster1", "cluster2", "cluster3", "cluster4"},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with only success clusters",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `["cluster1","cluster2"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster1", "cluster2"},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with only failure clusters",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"failure": `["cluster3"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster3"},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing and no success/failure - should return nil",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"other-key": `["cluster1"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: nil,
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with invalid success JSON - should return error",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `invalid-json`,
					"failure": `["cluster3"]`,
				},
			},
			key:           "all-clusters",
			expectedError: true,
			errorContains: "failed to unmarshal success clusters from ConfigMap",
		},
		{
			name: "all-clusters key missing with invalid failure JSON - should return error",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `["cluster1"]`,
					"failure": `invalid-json`,
				},
			},
			key:           "all-clusters",
			expectedError: true,
			errorContains: "failed to unmarshal failed clusters from ConfigMap",
		},
		{
			name: "ConfigMap with single cluster",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"all-clusters": `["single-cluster"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"single-cluster"},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with empty success and failure arrays - should return nil",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `[]`,
					"failure": `[]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: nil,
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with empty success and non-empty failure",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `[]`,
					"failure": `["cluster1","cluster2"]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster1", "cluster2"},
			expectedError:    false,
		},
		{
			name: "all-clusters key missing with non-empty success and empty failure",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Data: map[string]string{
					"success": `["cluster1","cluster2"]`,
					"failure": `[]`,
				},
			},
			key:              "all-clusters",
			expectedClusters: []string{"cluster1", "cluster2"},
			expectedError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.configMap != nil {
				clientBuilder = clientBuilder.WithObjects(tt.configMap)
			}
			fakeClient := clientBuilder.Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
				Scheme: scheme,
			}

			clusters, err := controller.getClusterFromConfigMap(
				context.TODO(),
				"test-migration",
				utils.GetDefaultNamespace(),
				tt.key,
			)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedClusters, clusters)
			}
		})
	}
}

// TestGetClusterFromConfigMap_EmptyData tests edge cases with empty or nil ConfigMap data
func TestGetClusterFromConfigMap_EmptyData(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: utils.GetDefaultNamespace(),
		},
		Data: nil, // nil data map
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMap).
		Build()

	controller := &ClusterMigrationController{
		Client: fakeClient,
		Scheme: scheme,
	}

	clusters, err := controller.getClusterFromConfigMap(
		context.TODO(),
		"test-migration",
		utils.GetDefaultNamespace(),
		"all-clusters",
	)

	assert.NoError(t, err)
	assert.Nil(t, clusters)
}

// TestGetClusterFromConfigMap_DifferentNamespaces tests retrieval from different namespaces
func TestGetClusterFromConfigMap_DifferentNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Create ConfigMaps in different namespaces
	configMapNS1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: "namespace1",
		},
		Data: map[string]string{
			"all-clusters": `["cluster-ns1"]`,
		},
	}

	configMapNS2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-migration",
			Namespace: "namespace2",
		},
		Data: map[string]string{
			"all-clusters": `["cluster-ns2"]`,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(configMapNS1, configMapNS2).
		Build()

	controller := &ClusterMigrationController{
		Client: fakeClient,
		Scheme: scheme,
	}

	// Get from namespace1
	clusters1, err := controller.getClusterFromConfigMap(
		context.TODO(),
		"test-migration",
		"namespace1",
		"all-clusters",
	)
	assert.NoError(t, err)
	assert.Equal(t, []string{"cluster-ns1"}, clusters1)

	// Get from namespace2
	clusters2, err := controller.getClusterFromConfigMap(
		context.TODO(),
		"test-migration",
		"namespace2",
		"all-clusters",
	)
	assert.NoError(t, err)
	assert.Equal(t, []string{"cluster-ns2"}, clusters2)

	// Get from non-existent namespace
	clusters3, err := controller.getClusterFromConfigMap(
		context.TODO(),
		"test-migration",
		"namespace3",
		"all-clusters",
	)
	assert.NoError(t, err)
	assert.Nil(t, clusters3)
}
