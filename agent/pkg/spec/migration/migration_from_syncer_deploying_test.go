// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestDeploying_emptyManagedClusters(t *testing.T) {
	configs.SetAgentConfig(&configs.AgentConfig{LeafHubName: "source-hub"})
	t.Cleanup(func() { configs.SetAgentConfig(nil) })

	scheme := configs.GetRuntimeScheme()
	syncer := &MigrationSourceSyncer{
		client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	err := syncer.deploying(context.Background(), &migration.MigrationSourceBundle{
		MigrationId:     "migration-1",
		ToHub:           "target-hub",
		ManagedClusters: []string{},
	})
	require.NoError(t, err)
}

func TestProcessResourceByType_ManagedCluster(t *testing.T) {
	syncer := &MigrationSourceSyncer{}
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "cluster.open-cluster-management.io/v1",
			"kind":       "ManagedCluster",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					constants.ManagedClusterMigrating: "true",
					KlusterletConfigAnnotation:      "config",
				},
			},
			"spec": map[string]interface{}{
				"managedClusterClientConfigs": []interface{}{map[string]interface{}{"url": "https://example"}},
			},
		},
	}

	syncer.processResourceByType(resource, MigrationResource{
		gvk: schema.GroupVersionKind{Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster"},
	})

	_, found, err := unstructured.NestedFieldNoCopy(resource.Object, "spec", "managedClusterClientConfigs")
	require.NoError(t, err)
	assert.False(t, found)
	annotations := resource.GetAnnotations()
	assert.NotContains(t, annotations, constants.ManagedClusterMigrating)
	assert.NotContains(t, annotations, KlusterletConfigAnnotation)
}

func TestProcessResourceByType_ClusterDeployment(t *testing.T) {
	syncer := &MigrationSourceSyncer{}
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "hive.openshift.io/v1",
			"kind":       "ClusterDeployment",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					HivePauseAnnotation: "true",
				},
			},
		},
	}

	syncer.processResourceByType(resource, MigrationResource{
		gvk: schema.GroupVersionKind{Group: "hive.openshift.io", Version: "v1", Kind: "ClusterDeployment"},
	})

	assert.NotContains(t, resource.GetAnnotations(), HivePauseAnnotation)
}

func TestProcessResourceByType_BareMetalHost(t *testing.T) {
	syncer := &MigrationSourceSyncer{}
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metal3.io/v1alpha1",
			"kind":       "BareMetalHost",
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{
					Metal3PauseAnnotation: "true",
				},
			},
		},
	}

	syncer.processResourceByType(resource, MigrationResource{
		gvk: schema.GroupVersionKind{Group: "metal3.io", Version: "v1alpha1", Kind: "BareMetalHost"},
	})

	assert.NotContains(t, resource.GetAnnotations(), Metal3PauseAnnotation)
}

func TestProcessResourceByType_ImageClusterInstall(t *testing.T) {
	syncer := &MigrationSourceSyncer{}
	resource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "extensions.hive.openshift.io/v1alpha1",
			"kind":       "ImageClusterInstall",
			"metadata":   map[string]interface{}{},
		},
	}

	syncer.processResourceByType(resource, MigrationResource{
		gvk: schema.GroupVersionKind{
			Group: "extensions.hive.openshift.io", Version: "v1alpha1", Kind: "ImageClusterInstall",
		},
	})

	assert.Equal(t, GlobalHubRestoreName, resource.GetLabels()[VeleroRestoreNameLabel])
}
