/*
Copyright (c) 2026 Red Hat, Inc.
Copyright Contributors to the Open Cluster Management project

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

func newAnnotatorTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.Install(scheme), "clusterv1 scheme registration should succeed")
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
	scheme := newAnnotatorTestScheme(t)
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
	require.NoError(t, err, "should retrieve annotated ManagedCluster ks after reconcile")
	assert.Equal(t, testKlusterletConfigName, mc.GetAnnotations()[klusterletConfigAnnotation],
		"newly imported ManagedCluster must get agent.open-cluster-management.io/klusterlet-config so HA standby bootstrap is applied")
}

func TestHAConfigAnnotator_NoOpWithoutKlusterletConfig(t *testing.T) {
	scheme := newAnnotatorTestScheme(t)
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
	require.NoError(t, err, "should retrieve no-op ManagedCluster ks to verify it was left unchanged")
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"ManagedCluster must not be annotated before ha-standby-* KlusterletConfig exists")
}

func TestHAConfigAnnotator_SkipsLocalClusterByLabel(t *testing.T) {
	scheme := newAnnotatorTestScheme(t)
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
	require.NoError(t, err, "reconcile of label-local cluster should succeed without annotating")

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: "acm-local-cluster"}, mc)
	require.NoError(t, err, "should retrieve label-local ManagedCluster to verify it remains unannotated")
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"local ManagedCluster with local-cluster=true must remain unannotated so the hub self-management cluster is not given HA standby klusterlet-config")
}

func TestHAConfigAnnotator_SkipsLocalClusterByName(t *testing.T) {
	scheme := newAnnotatorTestScheme(t)
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
	require.NoError(t, err, "reconcile of name-local cluster should succeed without annotating")

	mc := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Name: constants.LocalClusterName}, mc)
	require.NoError(t, err, "should retrieve name-local ManagedCluster to verify it remains unannotated")
	assert.Empty(t, mc.GetAnnotations()[klusterletConfigAnnotation],
		"ManagedCluster named local-cluster must remain unannotated so the conventional local hub cluster is not given HA standby klusterlet-config")
}

func TestHAConfigAnnotator_IdempotentWhenAlreadyAnnotated(t *testing.T) {
	scheme := newAnnotatorTestScheme(t)
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
	require.NoError(t, err, "should retrieve already-annotated ManagedCluster after idempotent reconcile")
	assert.Equal(t, testKlusterletConfigName, mc.GetAnnotations()[klusterletConfigAnnotation],
		"idempotent reconciliation must preserve the existing klusterlet-config annotation value")
}

func TestFindHAKlusterletConfigName(t *testing.T) {
	scheme := newAnnotatorTestScheme(t)

	t.Run("returns empty when none exist", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		name, err := findHAKlusterletConfigName(context.Background(), fakeClient)
		require.NoError(t, err, "listing KlusterletConfigs should succeed when none exist")
		assert.Empty(t, name, "expected empty name when no ha-standby-* KlusterletConfig exists")
	})

	t.Run("returns ha-standby prefixed name", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(newHAKlusterletConfig(testKlusterletConfigName)).
			Build()
		name, err := findHAKlusterletConfigName(context.Background(), fakeClient)
		require.NoError(t, err, "listing KlusterletConfigs should succeed when ha-standby-* exists")
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
		require.NoError(t, err, "listing KlusterletConfigs should succeed when only non-HA configs exist")
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
	scheme := newAnnotatorTestScheme(t)
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mc).Build()

	err := annotateManagedCluster(context.Background(), fakeClient, mc, testKlusterletConfigName)
	require.NoError(t, err, "annotateManagedCluster should succeed when setting klusterlet-config on a managed cluster")

	updated := &clusterv1.ManagedCluster{}
	err = fakeClient.Get(context.Background(), client.ObjectKeyFromObject(mc), updated)
	require.NoError(t, err, "should retrieve ManagedCluster after annotateManagedCluster to verify annotation was persisted")
	assert.Equal(t, testKlusterletConfigName, updated.GetAnnotations()[klusterletConfigAnnotation],
		"annotateManagedCluster should set klusterlet-config annotation")
}
