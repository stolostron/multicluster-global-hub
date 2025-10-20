// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestMigrationResourceBundleOperations tests the batch operations of MigrationResourceBundle
func TestMigrationResourceBundleOperations(t *testing.T) {
	cases := []struct {
		name               string
		resources          []MigrationClusterResource
		maxBundleSize      int
		expectedBundles    int
		expectedError      bool
		expectedSingleFail bool
	}{
		{
			name: "Single cluster fits in one bundle",
			resources: []MigrationClusterResource{
				{
					ClusterName: "cluster1",
					ResourceList: []unstructured.Unstructured{
						createTestResource("ManagedCluster", "cluster1"),
					},
				},
			},
			expectedBundles: 1,
			expectedError:   false,
		},
		{
			name: "Multiple clusters fit in one bundle",
			resources: []MigrationClusterResource{
				{
					ClusterName: "cluster1",
					ResourceList: []unstructured.Unstructured{
						createTestResource("ManagedCluster", "cluster1"),
					},
				},
				{
					ClusterName: "cluster2",
					ResourceList: []unstructured.Unstructured{
						createTestResource("ManagedCluster", "cluster2"),
					},
				},
			},
			expectedBundles: 1,
			expectedError:   false,
		},
		{
			name: "Empty bundle operations",
			resources: []MigrationClusterResource{
				{
					ClusterName:  "cluster1",
					ResourceList: []unstructured.Unstructured{},
				},
			},
			expectedBundles: 1,
			expectedError:   false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bundle := NewMigrationResourceBundle("test-migration-123")

			// Test initial state
			assert.True(t, bundle.IsEmpty())
			size, err := bundle.Size()
			assert.NoError(t, err)
			assert.Greater(t, size, 0) // Should have some size for migration ID JSON

			// Add resources
			for _, resource := range c.resources {
				added, err := bundle.AddClusterResource(resource)
				if c.expectedError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
					if len(c.resources) > 0 {
						assert.True(t, added)
					}
				}
			}

			// Test bundle state after adding resources
			if len(c.resources) > 0 && !c.expectedError {
				assert.False(t, bundle.IsEmpty())
			}

			// Test Size method
			size, err = bundle.Size()
			assert.NoError(t, err)
			assert.Greater(t, size, 0)
			assert.LessOrEqual(t, size, MaxMigrationBundleBytes)

			// Test Clean method
			bundle.Clean()
			assert.Nil(t, bundle.MigrationClusterResources)
		})
	}
}

// TestAddClusterResourceBoundary tests the boundary conditions of AddClusterResource
func TestAddClusterResourceBoundary(t *testing.T) {
	t.Run("Adding resource that exceeds size limit when bundle is empty should fail", func(t *testing.T) {
		bundle := NewMigrationResourceBundle("test-migration")

		// Create a very large resource with many resources
		largeResourceList := make([]unstructured.Unstructured, 0)
		// Add enough resources to exceed MaxMigrationBundleBytes
		for i := 0; i < 1000; i++ {
			resource := createTestResource("ManagedCluster", "test-cluster")
			// Add lots of data to make it large
			resource.Object["spec"] = map[string]interface{}{
				"data":     make([]byte, 1024), // 1KB per resource
				"metadata": "large data content to increase size",
			}
			largeResourceList = append(largeResourceList, resource)
		}

		clusterResource := MigrationClusterResource{
			ClusterName:  "large-cluster",
			ResourceList: largeResourceList,
		}

		added, err := bundle.AddClusterResource(clusterResource)
		assert.Error(t, err)
		assert.False(t, added)
		assert.Contains(t, err.Error(), "exceeds bundle size limit")
	})

	t.Run("Adding resource that would exceed size limit when bundle is not empty", func(t *testing.T) {
		bundle := NewMigrationResourceBundle("test-migration")

		// First add a normal-sized resource
		smallResource := MigrationClusterResource{
			ClusterName: "cluster1",
			ResourceList: []unstructured.Unstructured{
				createTestResource("ManagedCluster", "cluster1"),
			},
		}
		added, err := bundle.AddClusterResource(smallResource)
		assert.NoError(t, err)
		assert.True(t, added)

		// Create a resource that will cause the bundle to exceed the size limit
		largeResourceList := make([]unstructured.Unstructured, 0)
		for i := 0; i < 1000; i++ {
			resource := createTestResource("ManagedCluster", "test-cluster")
			resource.Object["spec"] = map[string]interface{}{
				"data": make([]byte, 1024),
			}
			largeResourceList = append(largeResourceList, resource)
		}

		largeResource := MigrationClusterResource{
			ClusterName:  "cluster2",
			ResourceList: largeResourceList,
		}

		added, err = bundle.AddClusterResource(largeResource)
		assert.NoError(t, err) // Should not error, just return false
		assert.False(t, added) // Should not be added
		// Verify the bundle still contains only the first resource
		assert.Equal(t, 1, len(bundle.MigrationClusterResources))
	})
}

// TestMigrationBundleBatchScenario tests a realistic batch scenario
func TestMigrationBundleBatchScenario(t *testing.T) {
	migrationID := "test-migration-456"
	totalClusters := 10

	bundle := NewMigrationResourceBundle(migrationID)
	bundleCount := 0
	addedClusters := 0

	for i := 0; i < totalClusters; i++ {
		clusterName := "cluster-" + string(rune('a'+i))
		resource := MigrationClusterResource{
			ClusterName: clusterName,
			ResourceList: []unstructured.Unstructured{
				createTestResource("ManagedCluster", clusterName),
				createTestResource("KlusterletAddonConfig", clusterName),
			},
		}

		added, err := bundle.AddClusterResource(resource)
		assert.NoError(t, err)

		if added {
			addedClusters++
		} else {
			// Bundle is full, "send" it and create a new one
			assert.False(t, bundle.IsEmpty())
			bundleCount++
			bundle = NewMigrationResourceBundle(migrationID)

			// Re-add the resource to the new bundle
			added, err = bundle.AddClusterResource(resource)
			assert.NoError(t, err)
			assert.True(t, added)
			addedClusters++
		}
	}

	// Send the last bundle if it's not empty
	if !bundle.IsEmpty() {
		bundleCount++
	}

	assert.Equal(t, totalClusters, addedClusters)
	assert.GreaterOrEqual(t, bundleCount, 1)
}

// createTestResource creates a test unstructured resource for testing
func createTestResource(kind, name string) unstructured.Unstructured {
	resource := unstructured.Unstructured{}
	resource.SetKind(kind)
	resource.SetAPIVersion("v1")
	resource.SetName(name)
	resource.SetNamespace(name)
	resource.Object["spec"] = map[string]interface{}{
		"hubAcceptsClient": true,
	}
	return resource
}
