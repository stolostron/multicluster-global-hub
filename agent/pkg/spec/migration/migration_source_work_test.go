// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestIsACM213(t *testing.T) {
	require.True(t, isACM213("2.13"))
	require.True(t, isACM213("2.13.0"))
	require.True(t, isACM213("v2.13.5"))
	require.False(t, isACM213("2.14.0"))
	require.False(t, isACM213("5.0.0"))
	require.False(t, isACM213("2.130.0"))
	require.False(t, isACM213("12.13.0"))
}

func TestSourceWorkMutationSupported(t *testing.T) {
	require.False(t, sourceWorkMutationSupported("2.13.0"))
	require.False(t, sourceWorkMutationSupported("2.14.0"))
	require.False(t, sourceWorkMutationSupported("2.16.1"))
	require.True(t, sourceWorkMutationSupported("2.17.0"))
	require.True(t, sourceWorkMutationSupported("v2.17.5"))
	require.True(t, sourceWorkMutationSupported("5.0.0"))
	require.False(t, sourceWorkMutationSupported(""))
}

func TestSourceWorkPollBudgetReservesCutover(t *testing.T) {
	ctx := withExpireTime(context.Background(), time.Now().Add(2*time.Minute))
	poll, err := sourceWorkPollBudget(ctx)
	require.NoError(t, err)
	require.Less(t, poll, 2*time.Minute)
	require.Greater(t, poll, time.Minute)

	expired := withExpireTime(context.Background(), time.Now().Add(-time.Second))
	_, err = sourceWorkPollBudget(expired)
	require.ErrorContains(t, err, "expired")

	short := withExpireTime(context.Background(), time.Now().Add(15*time.Second))
	_, err = sourceWorkPollBudget(short)
	require.ErrorContains(t, err, "insufficient time")
}

func TestInitializingRejectsUnsupportedWorkBeforeChanges(t *testing.T) {
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "5.0.0"},
	}
	ready := klusterletSourceWork("cluster1")
	hosted := klusterletSourceWork("cluster2")
	objects, ki, err := decodeSourceWork(hosted)
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(objects[ki].Object, "Hosted", "spec", "deployOption", "mode"))
	require.NoError(t, replaceWorkManifest(hosted, ki, objects[ki]))
	mc1 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
		Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}
	mc2 := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster2"},
		Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}
	c := sourceWorkClient(t, mch, ready, hosted, mc1, mc2)
	syncer := &MigrationSourceSyncer{client: c}
	event := sourceWorkEvent("cluster1", "cluster2")
	err = syncer.initializing(context.Background(), event)
	require.ErrorContains(t, err, "unsupported source work")
	require.ErrorContains(t, err, "cluster2")
	secret := &corev1.Secret{}
	require.True(t, apierrors.IsNotFound(c.Get(
		context.Background(), client.ObjectKeyFromObject(event.BootstrapSecret), secret,
	)))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc1))
	require.NotContains(t, mc1.Annotations, apiconstants.DisableAutoImportAnnotation)
}

func TestInitializingPreparesWorkAndRegisteringWaitsForApplied(t *testing.T) {
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "5.0.0"},
	}
	work := klusterletSourceWork("cluster1")
	originalManifests := len(work.Spec.Workload.Manifests)
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Annotations: map[string]string{"unrelated": "keep"}},
		Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}
	c := sourceWorkClient(t, mch, work, mc)
	syncer := &MigrationSourceSyncer{client: c}
	event := sourceWorkEvent("cluster1")
	require.NoError(t, syncer.initializing(context.Background(), event))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.True(t, mc.Spec.HubAcceptsClient)
	require.Contains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation,
		"initialization must keep Hive from rendering a second import")
	require.Contains(t, mc.Annotations, allowManifestWorkUpdateAnnotation,
		"initialization must open the source-work update window")
	require.Equal(t, "keep", mc.Annotations["unrelated"])
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.Contains(t, work.Annotations, sourceWorkBaselineAnnotation)
	require.Greater(t, len(work.Spec.Workload.Manifests), originalManifests)

	staleCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, syncer.registering(staleCtx, event))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.True(t, mc.Spec.HubAcceptsClient, "admission stays open until the new generation is Applied")

	readonly := work.DeepCopy()
	readonly.ResourceVersion = ""
	readonly.Generation = 4
	readonly.Status.Conditions = []metav1.Condition{{
		Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 4,
	}}
	readonly.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{
		UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly},
	}}
	require.NoError(t, c.Delete(context.Background(), work))
	require.NoError(t, c.Create(context.Background(), readonly))
	readonlyCtx := withExpireTime(context.Background(), time.Now().Add(22*time.Second))
	require.Error(t, syncer.registering(readonlyCtx, event))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.True(t, mc.Spec.HubAcceptsClient, "admission stays open while the work is ReadOnly")

	// The fake client does not preserve Generation across Update. Recreate the
	// work with the generation and Applied status the work agent would report.
	applied := work.DeepCopy()
	applied.ResourceVersion = ""
	applied.Generation = 4
	applied.Status.Conditions = []metav1.Condition{{
		Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 4,
	}}
	require.NoError(t, c.Delete(context.Background(), readonly))
	require.NoError(t, c.Create(context.Background(), applied))
	require.NoError(t, syncer.registering(context.Background(), event))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.False(t, mc.Spec.HubAcceptsClient)
	require.NotContains(t, mc.Annotations, allowManifestWorkUpdateAnnotation)
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	_, recorded := sourceWorkAppliedGeneration(work, event.MigrationId)
	require.True(t, recorded, "registration must record the writable Applied generation before locking")
}

func TestRegisteringRetriesAfterSourceWorkLock(t *testing.T) {
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "5.0.0"},
	}
	event := sourceWorkEvent("cluster1")
	work := appliedSourceWork("cluster1", "hub2", event.MigrationId)
	work.Generation = 5
	work.Status.Conditions[0].ObservedGeneration = 4
	work.Annotations[sourceWorkAppliedAnnotation] = sourceWorkAppliedValue(event.MigrationId, 4)
	work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{
		UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly},
	}}
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Annotations: map[string]string{
			apiconstants.DisableAutoImportAnnotation: "",
		}},
		Spec: clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}
	c := sourceWorkClient(t, mch, work, mc, event.BootstrapSecret)
	syncer := &MigrationSourceSyncer{client: c}
	require.NoError(t, syncer.registering(context.Background(), event))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.False(t, mc.Spec.HubAcceptsClient)
	require.NotContains(t, mc.Annotations, allowManifestWorkUpdateAnnotation)
}

func TestRollbackRestoresSourceWorkBeforeAnnotations(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	work := klusterletSourceWork("cluster1")
	require.NoError(t, prepareSourceWork(work, event, event.BootstrapSecret))
	work.Annotations[sourceWorkAppliedAnnotation] = sourceWorkAppliedValue(event.MigrationId, 2)
	before := work.DeepCopy()
	mc := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster1", Annotations: map[string]string{
			constants.ManagedClusterMigrating: "", KlusterletConfigAnnotation: "migration-hub2",
			apiconstants.DisableAutoImportAnnotation: "",
			allowManifestWorkUpdateAnnotation:        "",
		}},
		Spec: clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
	}
	c := sourceWorkClient(t, work, mc, event.BootstrapSecret)
	syncer := &MigrationSourceSyncer{client: c, clusterErrors: map[string]string{}}
	require.NoError(t, syncer.rollbackInitializing(context.Background(), event))
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.NotContains(t, work.Annotations, sourceWorkBaselineAnnotation)
	require.NotContains(t, work.Annotations, sourceWorkAppliedAnnotation)
	require.Equal(t, len(before.Spec.Workload.Manifests)-2, len(work.Spec.Workload.Manifests))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
	require.NotContains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
	require.NotContains(t, mc.Annotations, allowManifestWorkUpdateAnnotation)
}

func TestLegacyACMLeavesSourceWorkUnchanged(t *testing.T) {
	for _, version := range []string{"2.13.0", "2.14.0", "2.16.0"} {
		t.Run(version, func(t *testing.T) {
			mch := &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
				Status:     mchv1.MultiClusterHubStatus{CurrentVersion: version},
			}
			work := klusterletSourceWork("cluster1")
			manifests := len(work.Spec.Workload.Manifests)
			mc := &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: true},
			}
			c := sourceWorkClient(t, mch, work, mc)
			syncer := &MigrationSourceSyncer{client: c}
			event := sourceWorkEvent("cluster1")
			require.NoError(t, syncer.initializing(context.Background(), event))
			require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
			require.NotContains(t, work.Annotations, sourceWorkBaselineAnnotation)
			require.Equal(t, manifests, len(work.Spec.Workload.Manifests))
			secret := &corev1.Secret{}
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(event.BootstrapSecret), secret))
			require.NoError(t, syncer.registering(context.Background(), event))
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
			require.False(t, mc.Spec.HubAcceptsClient)
		})
	}
}

func sourceWorkClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.Install(scheme))
	require.NoError(t, workv1.Install(scheme))
	require.NoError(t, mchv1.AddToScheme(scheme))
	require.NoError(t, klusterletv1alpha1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&workv1.ManifestWork{}, &clusterv1.ManagedCluster{}).
		WithInterceptorFuncs(sourceWorkStrategyInterceptor()).
		WithObjects(objects...).Build()
}

func sourceWorkEvent(clusters ...string) *migration.MigrationSourceBundle {
	return &migration.MigrationSourceBundle{
		MigrationId:     "migration-1",
		ToHub:           "hub2",
		ManagedClusters: clusters,
		BootstrapSecret: targetBootstrapSecret("hub2"),
	}
}

func targetBootstrapSecret(toHub string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + toHub, Namespace: "multicluster-engine"},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"kubeconfig": []byte("target-kubeconfig")},
	}
}

func klusterletSourceWork(cluster string) *workv1.ManifestWork {
	kubeconfig := base64.StdEncoding.EncodeToString([]byte("current-hub-kubeconfig"))
	manifests := []map[string]interface{}{
		{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{"name": "keep-me", "namespace": "open-cluster-management"},
			"data":     map[string]interface{}{"k": "v"},
		},
		{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{"name": sourceBootstrapKubeconfigName, "namespace": spokeAgentNamespace},
			"type":     "Opaque",
			"data":     map[string]interface{}{"kubeconfig": kubeconfig},
		},
		{
			"apiVersion": "operator.open-cluster-management.io/v1", "kind": "Klusterlet",
			"metadata": map[string]interface{}{"name": "klusterlet"},
			"spec": map[string]interface{}{
				"clusterName": cluster, "namespace": spokeAgentNamespace,
				"deployOption": map[string]interface{}{"mode": "Default"},
			},
		},
	}
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: cluster + "-klusterlet", Namespace: cluster}}
	for _, manifest := range manifests {
		raw, err := json.Marshal(manifest)
		if err != nil {
			panic(err)
		}
		work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{
			RawExtension: runtime.RawExtension{Raw: raw},
		})
	}
	return work
}

func appliedSourceWork(cluster, toHub, migrationID string) *workv1.ManifestWork {
	work := klusterletSourceWork(cluster)
	event := &migration.MigrationSourceBundle{MigrationId: migrationID, ToHub: toHub}
	if err := prepareSourceWork(work, event, targetBootstrapSecret(toHub)); err != nil {
		panic(err)
	}
	work.Generation = 2
	work.Status.Conditions = []metav1.Condition{{
		Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 2,
	}}
	return work
}
