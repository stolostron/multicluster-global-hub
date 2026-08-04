// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
)

func TestFormatErrorMessages(t *testing.T) {
	assert.Empty(t, formatErrorMessages(nil))
	assert.Empty(t, formatErrorMessages(map[string]string{}))
	assert.Equal(t, "2 error(s), get more details in events", formatErrorMessages(map[string]string{
		"cluster1": "failed",
		"cluster2": "timeout",
	}))
}

func TestGetResourceFinalizerSuffix(t *testing.T) {
	assert.Equal(t, "/deprovision", getResourceFinalizerSuffix("ClusterDeployment"))
	assert.Equal(t, ".metal3.io", getResourceFinalizerSuffix("BareMetalHost"))
	assert.Equal(t, ".metal3.io", getResourceFinalizerSuffix("DataImage"))
	assert.Empty(t, getResourceFinalizerSuffix("ManagedCluster"))
}

func TestDeleteResourceIfExists_nilObject(t *testing.T) {
	err := deleteResourceIfExists(context.Background(), nil, nil, false)
	assert.NoError(t, err)
}

func TestDeleteResourceIfExists_notFound(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "missing", Namespace: "ns"},
	}
	err := deleteResourceIfExists(context.Background(), client, secret, false)
	assert.NoError(t, err)
}

func TestDeleteResourceIfExists_deletesExistingResource(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "to-delete", Namespace: "ns"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	err := deleteResourceIfExists(context.Background(), client, secret, false)
	require.NoError(t, err)

	got := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "to-delete", Namespace: "ns"}, got)
	assert.Error(t, err)
}

func TestDeleteResourceIfExists_forceDeleteRemovesFinalizers(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "blocked",
			Namespace:  "ns",
			Finalizers: []string{"example.com/block"},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()

	err := deleteResourceIfExists(context.Background(), client, secret, true)
	require.NoError(t, err)

	got := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "blocked", Namespace: "ns"}, got)
	assert.Error(t, err)
}

func TestDeleteBMHAndDependentResources_skipsNonBMH(t *testing.T) {
	syncer := &MigrationTargetSyncer{}
	obj := &unstructured.Unstructured{}
	obj.SetKind("ClusterDeployment")
	err := syncer.deleteBMHAndDependentResources(context.Background(), obj)
	assert.NoError(t, err)
}

func TestDeleteBMHAndDependentResources_noCredentialsName(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	bmh := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metal3.io/v1alpha1",
			"kind":       "BareMetalHost",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "cluster1",
			},
			"spec": map[string]interface{}{},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bmh).Build()
	syncer := &MigrationTargetSyncer{client: client}

	err := syncer.deleteBMHAndDependentResources(context.Background(), bmh)
	assert.NoError(t, err)
}

func TestDeleteBMHAndDependentResources_deletesCredentialsSecret(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	bmh := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "metal3.io/v1alpha1",
			"kind":       "BareMetalHost",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "cluster1",
			},
			"spec": map[string]interface{}{
				"bmc": map[string]interface{}{
					"credentialsName": "bmc-secret",
				},
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bmc-secret", Namespace: "cluster1"},
	}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bmh, secret).Build()
	syncer := &MigrationTargetSyncer{client: client}

	err := syncer.deleteBMHAndDependentResources(context.Background(), bmh)
	require.NoError(t, err)

	got := &corev1.Secret{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "bmc-secret", Namespace: "cluster1"}, got)
	assert.Error(t, err)
}

func TestRemoveVeleroRestoreLabelFromImageClusterInstall(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	ici := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "extensions.hive.openshift.io/v1alpha1",
			"kind":       "ImageClusterInstall",
			"metadata": map[string]interface{}{
				"name":      "cluster1",
				"namespace": "cluster1",
				"labels": map[string]interface{}{
					VeleroRestoreNameLabel: GlobalHubRestoreName,
				},
			},
		},
	}
	ici.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "extensions.hive.openshift.io",
		Version: "v1alpha1",
		Kind:    "ImageClusterInstall",
	})
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ici).Build()
	syncer := &MigrationTargetSyncer{client: client}

	err := syncer.removeVeleroRestoreLabelFromImageClusterInstall(context.Background(), "cluster1")
	require.NoError(t, err)

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(ici.GroupVersionKind())
	err = client.Get(context.Background(), types.NamespacedName{Name: "cluster1", Namespace: "cluster1"}, got)
	require.NoError(t, err)
	_, hasLabel := got.GetLabels()[VeleroRestoreNameLabel]
	assert.False(t, hasLabel)
}

func TestRemoveVeleroRestoreLabelFromImageClusterInstall_notFound(t *testing.T) {
	scheme := configs.GetRuntimeScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	syncer := &MigrationTargetSyncer{client: client}

	err := syncer.removeVeleroRestoreLabelFromImageClusterInstall(context.Background(), "cluster1")
	assert.NoError(t, err)
}
