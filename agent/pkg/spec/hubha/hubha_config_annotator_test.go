// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package hubha

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func newAnnotatorTestScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = clusterv1.Install(scheme)
	return scheme
}

func newHAKlusterletConfig(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(klusterletConfigGVK)
	obj.SetName(name)
	obj.Object["spec"] = map[string]interface{}{}
	return obj
}

func TestHAConfigAnnotator_AnnotatesNewlyImportedCluster(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	ksCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "ks"},
	}
	klusterletConfig := newHAKlusterletConfig(testKlusterletConfigName)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ksCluster, klusterletConfig).
		Build()

	_, err := (&haConfigAnnotator{client: fakeClient}).Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "ks"},
	})
	require.NoError(t, err, "annotator should succeed for a newly imported cluster when HA KlusterletConfig exists")

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "ks"}, mc)
	require.NoError(t, err)
	assert.Equal(t, testKlusterletConfigName, mc.GetAnnotations()[klusterletConfigAnnotation],
		"newly imported ManagedCluster must get agent.open-cluster-management.io/klusterlet-config")
}

func TestHAConfigAnnotator_NoOpWithoutKlusterletConfig(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	ksCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "ks"},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ksCluster).
		Build()

	_, err := (&haConfigAnnotator{client: fakeClient}).Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "ks"},
	})
	require.NoError(t, err, "annotator should no-op when HA has not been configured yet")

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "ks"}, mc)
	require.NoError(t, err)
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"ManagedCluster must not be annotated before ha-standby-* KlusterletConfig exists")
}

func TestHAConfigAnnotator_SkipsLocalClusterByLabel(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "acm-local-cluster",
			Labels: map[string]string{
				constants.LocalClusterName: "true",
			},
		},
	}
	klusterletConfig := newHAKlusterletConfig(testKlusterletConfigName)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(localCluster, klusterletConfig).
		Build()

	_, err := (&haConfigAnnotator{client: fakeClient}).Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "acm-local-cluster"},
	})
	require.NoError(t, err)

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "acm-local-cluster"}, mc)
	require.NoError(t, err)
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"local ManagedCluster (local-cluster=true) must not get klusterlet-config annotation")
}

func TestHAConfigAnnotator_SkipsLocalClusterByName(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	localCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: constants.LocalClusterName},
	}
	klusterletConfig := newHAKlusterletConfig(testKlusterletConfigName)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(localCluster, klusterletConfig).
		Build()

	_, err := (&haConfigAnnotator{client: fakeClient}).Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: constants.LocalClusterName},
	})
	require.NoError(t, err)

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: constants.LocalClusterName}, mc)
	require.NoError(t, err)
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"ManagedCluster named local-cluster must not get klusterlet-config annotation")
}

func TestHAConfigAnnotator_IdempotentWhenAlreadyAnnotated(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	ksCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ks",
			Annotations: map[string]string{
				klusterletConfigAnnotation: testKlusterletConfigName,
			},
		},
	}
	klusterletConfig := newHAKlusterletConfig(testKlusterletConfigName)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ksCluster, klusterletConfig).
		Build()

	_, err := (&haConfigAnnotator{client: fakeClient}).Reconcile(context.Background(), reconcile.Request{
		NamespacedName: types.NamespacedName{Name: "ks"},
	})
	require.NoError(t, err, "annotator should be idempotent when annotation already matches")

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "ks"}, mc)
	require.NoError(t, err)
	assert.Equal(t, testKlusterletConfigName, mc.GetAnnotations()[klusterletConfigAnnotation])
}

func TestFindHAKlusterletConfigName(t *testing.T) {
	scheme := newAnnotatorTestScheme()

	t.Run("returns empty when none exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		name, err := findHAKlusterletConfigName(context.Background(), fakeClient)
		require.NoError(t, err)
		assert.Empty(t, name, "expected empty name when no ha-standby-* KlusterletConfig exists")
	})

	t.Run("returns ha-standby prefixed name", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(newHAKlusterletConfig(testKlusterletConfigName)).
			Build()
		name, err := findHAKlusterletConfigName(context.Background(), fakeClient)
		require.NoError(t, err)
		assert.Equal(t, testKlusterletConfigName, name,
			"should resolve KlusterletConfig with ha-standby- prefix")
	})

	t.Run("ignores non HA KlusterletConfig", func(t *testing.T) {
		other := newHAKlusterletConfig("grpc-config")
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(other).
			Build()
		name, err := findHAKlusterletConfigName(context.Background(), fakeClient)
		require.NoError(t, err)
		assert.Empty(t, name, "non ha-standby-* KlusterletConfig must be ignored")
	})
}

func TestIsLocalManagedCluster(t *testing.T) {
	assert.True(t, isLocalManagedCluster(&clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: constants.LocalClusterName},
	}), "name local-cluster should be treated as local")

	assert.True(t, isLocalManagedCluster(&clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "acm-local-cluster",
			Labels: map[string]string{
				constants.LocalClusterName: "true",
			},
		},
	}), "local-cluster=true label should be treated as local")

	assert.False(t, isLocalManagedCluster(&clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "ks"},
	}), "regular managed cluster should not be treated as local")
}

func TestAnnotateManagedCluster(t *testing.T) {
	scheme := newAnnotatorTestScheme()
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mc).Build()

	err := annotateManagedCluster(context.Background(), fakeClient, mc, testKlusterletConfigName)
	require.NoError(t, err)

	updated := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(mc), updated)
	require.NoError(t, err)
	assert.Equal(t, testKlusterletConfigName, updated.GetAnnotations()[klusterletConfigAnnotation],
		"annotateManagedCluster should set klusterlet-config annotation")
}
