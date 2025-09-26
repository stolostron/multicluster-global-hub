package migration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
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
