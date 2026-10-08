// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
)

// Synthetic fixture based on the sanitized ACM/MCE 5.0 source Hive work.
// It deliberately contains no live credential bytes or image-pull credentials.
func migrationSourceWorkFixture(t *testing.T, cluster string) *workv1.ManifestWork {
	t.Helper()
	objects := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]interface{}{"name": spokeAgentNamespace}},
		{
			"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{
				"name": sourceBootstrapSecretName, "namespace": spokeAgentNamespace,
			}, "type": "Opaque",
			"data": map[string]interface{}{"kubeconfig": base64.StdEncoding.EncodeToString([]byte("synthetic-source"))},
		},
		{
			"apiVersion": "operator.open-cluster-management.io/v1", "kind": "Klusterlet",
			"metadata": map[string]interface{}{"name": "klusterlet", "labels": map[string]interface{}{"keep": "yes"}},
			"spec": map[string]interface{}{
				"clusterName": cluster, "namespace": spokeAgentNamespace,
				"deployOption": map[string]interface{}{"mode": "Singleton"}, "priorityClassName": "klusterlet-critical",
				"registrationConfiguration": map[string]interface{}{
					"bootstrapKubeConfigs":      map[string]interface{}{},
					"featureGates":              []interface{}{map[string]interface{}{"feature": "NetworkPolicies", "mode": "Enable"}},
					"clusterClaimConfiguration": map[string]interface{}{"reservedClusterClaimSuffixes": []interface{}{"openshift.io"}},
				},
			},
		},
		{
			"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "unrelated", "namespace": spokeAgentNamespace},
			"data": map[string]interface{}{"keep": "yes"},
		},
	}
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
		Name: cluster + "-klusterlet", Namespace: cluster, Generation: 3,
		Annotations:     map[string]string{"unrelated": "keep"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: clusterv1.GroupVersion.String(), Kind: "ManagedCluster", Name: cluster}},
	}}
	for _, obj := range objects {
		raw, err := json.Marshal(obj)
		require.NoError(t, err)
		work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}})
	}
	work.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 3}}
	return work
}

func migrationTargetSecretFixture() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-hub2", Namespace: "multicluster-engine"},
		Data:       map[string][]byte{"kubeconfig": []byte("synthetic-target")},
	}
}

func sourceWorkEvent(clusters ...string) *migration.MigrationSourceBundle {
	return &migration.MigrationSourceBundle{
		MigrationId: "test-migration", ToHub: "hub2", ManagedClusters: clusters,
		BootstrapSecret: migrationTargetSecretFixture(),
	}
}

func sourceWorkClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, coordinationv1.AddToScheme(scheme))
	require.NoError(t, klusterletv1alpha1.AddToScheme(scheme))
	require.NoError(t, clusterv1.Install(scheme))
	require.NoError(t, workv1.Install(scheme))
	require.NoError(t, mchv1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&workv1.ManifestWork{}, &clusterv1.ManagedCluster{}, &mchv1.MultiClusterHub{}).
		WithObjects(objects...).Build()
}

func sourceMC(cluster string, accepted bool) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{Name: cluster},
		Spec:       clusterv1.ManagedClusterSpec{HubAcceptsClient: accepted},
		Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{
			Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue,
		}}},
	}
}

// Fake clients do not increment generation. Model spec generation changes and,
// when requested by a test, an explicit work-agent status acknowledgement.
type sourceWorkTestClient struct {
	client.Client
	workUpdates      int
	conflictOnce     bool
	autoApply        bool
	admissionUpdates int
	workUpdateError  error
}

func (c *sourceWorkTestClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if work, ok := obj.(*workv1.ManifestWork); ok {
		if c.workUpdateError != nil {
			return c.workUpdateError
		}
		latest := &workv1.ManifestWork{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(work), latest); err != nil {
			return err
		}
		if c.conflictOnce {
			c.conflictOnce = false
			latest.Annotations["concurrent"] = "preserved"
			if err := c.Client.Update(ctx, latest); err != nil {
				return err
			}
			return apierrors.NewConflict(schema.GroupResource{Group: workv1.GroupVersion.Group, Resource: "manifestworks"}, work.Name, fmt.Errorf("synthetic conflict"))
		}
		if !reflect.DeepEqual(latest.Spec, work.Spec) {
			work.Generation = latest.Generation + 1
		}
		if err := c.Client.Update(ctx, work, opts...); err != nil {
			return err
		}
		c.workUpdates++
		if c.autoApply {
			work.Status.Conditions = []metav1.Condition{{
				Type: workv1.WorkApplied, Status: metav1.ConditionTrue,
				ObservedGeneration: work.Generation,
			}}
			return c.Client.Status().Update(ctx, work)
		}
		return nil
	}
	if _, ok := obj.(*clusterv1.ManagedCluster); ok {
		c.admissionUpdates++
	}
	return c.Client.Update(ctx, obj, opts...)
}

func TestMigrationSourceWorkMutation(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	work := migrationSourceWorkFixture(t, "cluster1")
	work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{
		ResourceIdentifier: workv1.ResourceIdentifier{
			Resource: "configmaps", Name: "unrelated", Namespace: spokeAgentNamespace,
		},
		UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly},
	}}
	before := work.DeepCopy()
	c := &sourceWorkTestClient{Client: sourceWorkClient(t, work), conflictOnce: true}
	syncer := &MigrationSourceSyncer{client: c}
	require.NoError(t, syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret))
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.Equal(t, int64(4), work.Generation)
	require.Equal(t, before.OwnerReferences, work.OwnerReferences)
	require.Equal(t, before.Spec.ManifestConfigs, work.Spec.ManifestConfigs)
	require.Equal(t, "preserved", work.Annotations["concurrent"])
	require.Equal(t, "keep", work.Annotations["unrelated"])
	for _, index := range []int{0, 1, 3} {
		require.Equal(t, before.Spec.Workload.Manifests[index], work.Spec.Workload.Manifests[index])
	}
	objects, ki, err := decodeSourceWork(work)
	require.NoError(t, err)
	require.Len(t, objects, 6)
	ti := secretManifestIndex(objects, "bootstrap-hub2")
	require.NotEqual(t, -1, ti)
	encoded, _, err := unstructured.NestedString(objects[ti].Object, "data", "kubeconfig")
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(event.BootstrapSecret.Data["kubeconfig"]), encoded)
	refs, _, err := unstructured.NestedSlice(objects[ki].Object, "spec", "registrationConfiguration", "bootstrapKubeConfigs", "localSecretsConfig", "kubeConfigSecrets")
	require.NoError(t, err)
	require.Equal(t, []interface{}{map[string]interface{}{"name": "bootstrap-hub2"}, map[string]interface{}{"name": currentHubBootstrapSecretName}}, refs)
	ci := secretManifestIndex(objects, currentHubBootstrapSecretName)
	require.NotEqual(t, -1, ci)
	currentHubData, _, err := unstructured.NestedString(objects[ci].Object, "data", "kubeconfig")
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("synthetic-source")), currentHubData)
	gates, _, err := unstructured.NestedSlice(objects[ki].Object, "spec", "registrationConfiguration", "featureGates")
	require.NoError(t, err)
	require.Len(t, gates, 2)
	require.Equal(t, map[string]interface{}{"feature": "MultipleHubs", "mode": "Enable"}, gates[1])
	require.NotContains(t, work.Annotations[sourceWorkBaselineAnnotation], "synthetic")
	require.NotContains(t, work.Annotations[sourceWorkBaselineAnnotation], "kubeconfig")
	updates := c.workUpdates
	require.NoError(t, syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret))
	require.Equal(t, updates, c.workUpdates, "idempotent initialization must not bump generation")
	event.BootstrapSecret.Data["kubeconfig"] = []byte("synthetic-rotated-target")
	require.NoError(t, syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret))
	require.Equal(t, updates+1, c.workUpdates)
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.Len(t, work.Spec.Workload.Manifests, 6)
}

func TestMigrationSourceWorkInvalidPayload(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*workv1.ManifestWork, *migration.MigrationSourceBundle)
	}{
		{"missing Klusterlet", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.Workload.Manifests = w.Spec.Workload.Manifests[:2]
		}},
		{"malformed manifest", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.Workload.Manifests[0].Raw = []byte("not json")
		}},
		{"missing target kubeconfig", func(_ *workv1.ManifestWork, e *migration.MigrationSourceBundle) { e.BootstrapSecret.Data = nil }},
		{"missing source bootstrap", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.Workload.Manifests[1].Raw = []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"other"}}`)
		}},
		{"ambiguous Klusterlet", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.Workload.Manifests = append(w.Spec.Workload.Manifests, w.Spec.Workload.Manifests[2])
		}},
		{"CreateOnly Klusterlet", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{ResourceIdentifier: workv1.ResourceIdentifier{Group: "operator.open-cluster-management.io", Resource: "klusterlets", Name: "klusterlet"}, UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeCreateOnly}}}
		}},
		{"ReadOnly target Secret", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{ResourceIdentifier: workv1.ResourceIdentifier{Resource: "secrets", Namespace: spokeAgentNamespace, Name: "bootstrap-hub2"}, UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly}}}
		}},
		{"ReadOnly with inferred resource", func(w *workv1.ManifestWork, _ *migration.MigrationSourceBundle) {
			w.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{
				ResourceIdentifier: workv1.ResourceIdentifier{
					Group: "operator.open-cluster-management.io", Name: "klusterlet",
				},
				UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly},
			}}
		}},
		{"different migration", func(w *workv1.ManifestWork, e *migration.MigrationSourceBundle) {
			require.NoError(t, prepareSourceWork(w, e, e.BootstrapSecret))
			e.MigrationId = "other"
		}},
		{"unowned target Secret", func(w *workv1.ManifestWork, e *migration.MigrationSourceBundle) {
			require.NoError(t, prepareSourceWork(w, e, e.BootstrapSecret))
			delete(w.Annotations, sourceWorkBaselineAnnotation)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := migrationSourceWorkFixture(t, "cluster1")
			event := sourceWorkEvent("cluster1")
			tc.mutate(work, event)
			mc := sourceMC("cluster1", true)
			c := sourceWorkClient(t, work, mc)
			syncer := &MigrationSourceSyncer{client: c}
			err := syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "synthetic-target")
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
			require.True(t, mc.Spec.HubAcceptsClient)
		})
	}
}

func TestMigrationSourceRegisteringGate(t *testing.T) {
	for _, state := range []string{"ready", "stale", "missing", "false", "unknown", "no condition", "wrong payload", "already false stale", "already false valid", "expired", "canceled", "missing target Secret"} {
		t.Run(state, func(t *testing.T) {
			event := sourceWorkEvent("cluster1", "cluster2")
			w1 := migrationSourceWorkFixture(t, "cluster1")
			w2 := migrationSourceWorkFixture(t, "cluster2")
			for _, w := range []*workv1.ManifestWork{w1, w2} {
				require.NoError(t, prepareSourceWork(w, event, event.BootstrapSecret))
				w.Generation = 4
				w.Status.Conditions[0].ObservedGeneration = 4
			}
			mc1, mc2 := sourceMC("cluster1", true), sourceMC("cluster2", true)
			switch state {
			case "stale", "already false stale":
				w2.Status.Conditions[0].ObservedGeneration = 3
			case "false":
				w2.Status.Conditions[0].Status = metav1.ConditionFalse
			case "unknown":
				w2.Status.Conditions[0].Status = metav1.ConditionUnknown
			case "no condition":
				w2.Status.Conditions = nil
			case "wrong payload":
				w2.Spec.Workload.Manifests = w2.Spec.Workload.Manifests[:4]
			}
			if strings.HasPrefix(state, "already false") {
				mc1.Spec.HubAcceptsClient = false
				mc2.Spec.HubAcceptsClient = false
			}
			objects := []client.Object{w1, mc1, mc2}
			if state != "missing target Secret" {
				objects = append(objects, event.BootstrapSecret)
			}
			if state != "missing" {
				objects = append(objects, w2)
			}
			c := &sourceWorkTestClient{Client: sourceWorkClient(t, objects...)}
			syncer := &MigrationSourceSyncer{client: c}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			if state == "expired" {
				ctx = withExpireTime(ctx, time.Now().Add(-time.Second))
			}
			if state == "canceled" {
				cancel()
			}
			err := syncer.registering(ctx, event)
			if state == "ready" || state == "already false valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Zero(t, c.admissionUpdates, "neither cluster may be cut off by a failed all-clusters gate")
			}
			for _, mc := range []*clusterv1.ManagedCluster{mc1, mc2} {
				require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
				if state == "ready" || strings.HasPrefix(state, "already false") {
					require.False(t, mc.Spec.HubAcceptsClient)
				} else {
					require.True(t, mc.Spec.HubAcceptsClient)
				}
			}
		})
	}
}

func TestMigrationSourceWorkRollback(t *testing.T) {
	for _, stage := range []string{migrationv1alpha1.PhaseInitializing, migrationv1alpha1.PhaseDeploying, migrationv1alpha1.PhaseRegistering} {
		t.Run(stage, func(t *testing.T) {
			event := sourceWorkEvent("cluster1")
			event.RollbackStage = stage
			original := migrationSourceWorkFixture(t, "cluster1")
			work := original.DeepCopy()
			require.NoError(t, prepareSourceWork(work, event, event.BootstrapSecret))
			mc := sourceMC("cluster1", stage != migrationv1alpha1.PhaseRegistering)
			mc.Annotations = map[string]string{apiconstants.DisableAutoImportAnnotation: "", KlusterletConfigAnnotation: "migration-hub2"}
			c := &sourceWorkTestClient{Client: sourceWorkClient(t, work, mc, event.BootstrapSecret), autoApply: true}
			syncer := &MigrationSourceSyncer{client: c, clusterErrors: map[string]string{}}
			require.NoError(t, syncer.rollbacking(context.Background(), event))
			require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
			require.Equal(t, original.Spec, work.Spec)
			require.Equal(t, original.Annotations, work.Annotations)
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
			require.True(t, mc.Spec.HubAcceptsClient)
			require.NotContains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
			require.NoError(t, syncer.rollbacking(context.Background(), event), "repeated rollback is safe")
		})
	}
}

func TestMigrationSourceWorkRollbackWaitsForApplied(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	work := migrationSourceWorkFixture(t, "cluster1")
	require.NoError(t, prepareSourceWork(work, event, event.BootstrapSecret))
	work.Generation = 4
	mc := sourceMC("cluster1", true)
	mc.Annotations = map[string]string{apiconstants.DisableAutoImportAnnotation: ""}
	c := &sourceWorkTestClient{Client: sourceWorkClient(t, work, mc, event.BootstrapSecret)}
	syncer := &MigrationSourceSyncer{client: c}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.Error(t, syncer.rollbackInitializing(ctx, event))
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.Contains(t, work.Annotations, sourceWorkBaselineAnnotation, "retain restoration journal across timeout")
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
	require.Contains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation, "do not resume import controller before restoration Applied")
	c.autoApply = true
	work.Status.Conditions[0].ObservedGeneration = work.Generation
	require.NoError(t, c.Status().Update(context.Background(), work))
	require.NoError(t, syncer.rollbackInitializing(context.Background(), event))
}

func TestMigrationSourceWorkUnsupported213(t *testing.T) {
	mch := &mchv1.MultiClusterHub{
		ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
		Status:     mchv1.MultiClusterHubStatus{CurrentVersion: "2.13.0"},
	}
	c := sourceWorkClient(t, mch, sourceMC("cluster1", true))
	syncer := &MigrationSourceSyncer{client: c}
	err := syncer.initializing(context.Background(), sourceWorkEvent("cluster1"))
	require.ErrorContains(t, err, "2.13 is unsupported")
	secret := &corev1.Secret{}
	require.True(t, apierrors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(migrationTargetSecretFixture()), secret)))
}

func TestMigrationSourceWorkAgentRBAC(t *testing.T) {
	for _, path := range []string{
		"../../../../operator/pkg/controllers/agent/manifests/clusterrole.yaml",
		"../../../../operator/pkg/controllers/agent/addon/manifests/templates/agent/multicluster-global-hub-agent-clusterrole.yaml",
	} {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		role := &rbacv1.ClusterRole{}
		require.NoError(t, yaml.Unmarshal(content, role))
		found := false
		for _, rule := range role.Rules {
			if len(rule.APIGroups) == 1 && rule.APIGroups[0] == workv1.GroupVersion.Group &&
				len(rule.Resources) == 1 && rule.Resources[0] == "manifestworks" {
				require.Contains(t, rule.Verbs, "update", path)
				require.NotContains(t, rule.Verbs, "patch", path)
				found = true
			}
		}
		require.True(t, found, path)
	}
}

func TestMigrationSourceWorkInitializing(t *testing.T) {
	for _, version := range []string{"2.14.0", "5.0.0"} {
		t.Run(version, func(t *testing.T) {
			mch := &mchv1.MultiClusterHub{
				ObjectMeta: metav1.ObjectMeta{Name: "multiclusterhub"},
				Status:     mchv1.MultiClusterHubStatus{CurrentVersion: version},
			}
			work := migrationSourceWorkFixture(t, "cluster1")
			mc := sourceMC("cluster1", true)
			mc.Annotations = map[string]string{"unrelated": "keep"}
			c := &sourceWorkTestClient{Client: sourceWorkClient(t, mch, work, mc)}
			syncer := &MigrationSourceSyncer{client: c}
			event := sourceWorkEvent("cluster1")
			require.NoError(t, syncer.initializing(context.Background(), event))
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
			require.True(t, mc.Spec.HubAcceptsClient)
			require.Contains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
			require.Equal(t, "keep", mc.Annotations["unrelated"])
			require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
			require.Equal(t, int64(4), work.Generation)
			updates := c.workUpdates
			require.NoError(t, syncer.initializing(context.Background(), event))
			require.Equal(t, updates, c.workUpdates)
			// Spec was updated, but the old generation is still Applied. Admission
			// stays open until the work agent acknowledges the new generation.
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			require.Error(t, syncer.registering(ctx, event))
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc.Name}, mc))
			require.True(t, mc.Spec.HubAcceptsClient)
		})
	}
}

func TestMigrationSourceWorkPreservesExistingFallback(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	original := migrationSourceWorkFixture(t, "cluster1")
	objects, ki, err := decodeSourceWork(original)
	require.NoError(t, err)
	currentHub := objects[1].DeepCopy()
	currentHub.SetName(currentHubBootstrapSecretName)
	other := objects[1].DeepCopy()
	other.SetName("other-bootstrap")
	for _, secret := range []*unstructured.Unstructured{currentHub, other} {
		original.Spec.Workload.Manifests = append(original.Spec.Workload.Manifests, workv1.Manifest{})
		require.NoError(t, replaceWorkManifest(original, len(original.Spec.Workload.Manifests)-1, secret))
	}
	bootstrap := map[string]interface{}{"type": "LocalSecrets", "localSecretsConfig": map[string]interface{}{
		"hubConnectionTimeoutSeconds": int64(900),
		"kubeConfigSecrets": []interface{}{
			map[string]interface{}{"name": "other-bootstrap"},
			map[string]interface{}{"name": currentHubBootstrapSecretName},
		},
	}}
	require.NoError(t, unstructured.SetNestedMap(objects[ki].Object, bootstrap, "spec", "registrationConfiguration", "bootstrapKubeConfigs"))
	originalGates := []interface{}{
		map[string]interface{}{"feature": "NetworkPolicies", "mode": "Enable"},
		map[string]interface{}{"feature": "AddonManagement"},
	}
	require.NoError(t, unstructured.SetNestedSlice(objects[ki].Object, originalGates, "spec", "registrationConfiguration", "featureGates"))
	require.NoError(t, replaceWorkManifest(original, ki, objects[ki]))
	work := original.DeepCopy()
	c := &sourceWorkTestClient{Client: sourceWorkClient(t, work, sourceMC("cluster1", true)), autoApply: true}
	syncer := &MigrationSourceSyncer{client: c}
	require.NoError(t, syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret))
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	objects, ki, err = decodeSourceWork(work)
	require.NoError(t, err)
	localConfig, _, err := unstructured.NestedMap(objects[ki].Object, "spec", "registrationConfiguration", "bootstrapKubeConfigs", "localSecretsConfig")
	require.NoError(t, err)
	require.Equal(t, float64(900), localConfig["hubConnectionTimeoutSeconds"])
	gates, _, err := unstructured.NestedSlice(objects[ki].Object, "spec", "registrationConfiguration", "featureGates")
	require.NoError(t, err)
	require.Equal(t, originalGates, gates[:2], "preserve unrelated feature gates including omitted default mode")
	refs, _, err := unstructured.NestedSlice(objects[ki].Object, "spec", "registrationConfiguration", "bootstrapKubeConfigs", "localSecretsConfig", "kubeConfigSecrets")
	require.NoError(t, err)
	require.Equal(t, []interface{}{
		map[string]interface{}{"name": "bootstrap-hub2"},
		map[string]interface{}{"name": "other-bootstrap"},
		map[string]interface{}{"name": currentHubBootstrapSecretName},
	}, refs)
	require.NoError(t, syncer.restoreMigrationSourceWork(context.Background(), "cluster1", event))
	require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
	require.Equal(t, original.Spec, work.Spec, "retain preexisting fallback Secrets on rollback")
}

func TestMigrationSourceWorkWriteErrorsAreSafe(t *testing.T) {
	for _, reason := range []metav1.StatusReason{metav1.StatusReasonForbidden, metav1.StatusReasonInvalid} {
		t.Run(string(reason), func(t *testing.T) {
			work := migrationSourceWorkFixture(t, "cluster1")
			original := work.DeepCopy()
			apiErr := &apierrors.StatusError{ErrStatus: metav1.Status{
				Reason: reason, Code: 403,
				Message: "rejected raw manifest with synthetic-target credentials",
			}}
			c := &sourceWorkTestClient{Client: sourceWorkClient(t, work, sourceMC("cluster1", true)), workUpdateError: apiErr}
			syncer := &MigrationSourceSyncer{client: c}
			event := sourceWorkEvent("cluster1")
			err := syncer.ensureMigrationSourceWork(context.Background(), "cluster1", event, event.BootstrapSecret)
			require.ErrorContains(t, err, string(reason))
			require.NotContains(t, err.Error(), "synthetic-target")
			require.NoError(t, c.Get(context.Background(), sourceWorkKey("cluster1"), work))
			require.Equal(t, original.Spec, work.Spec)
			mc := &clusterv1.ManagedCluster{}
			require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: "cluster1"}, mc))
			require.True(t, mc.Spec.HubAcceptsClient)
		})
	}
}

func TestMigrationSourceWorkRollbackUsesTargetHub(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	targetSecret := event.BootstrapSecret.DeepCopy()
	sourceSecret := targetSecret.DeepCopy()
	sourceSecret.Name = "bootstrap-hub1"
	// Match the old manager's rollback payload, which names the source hub.
	event.BootstrapSecret = sourceSecret
	c := sourceWorkClient(t, sourceMC("cluster1", true), sourceSecret, targetSecret)
	syncer := &MigrationSourceSyncer{client: c}
	require.NoError(t, syncer.rollbackInitializing(context.Background(), event))
	require.True(t, apierrors.IsNotFound(c.Get(context.Background(), client.ObjectKeyFromObject(targetSecret), &corev1.Secret{})))
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(sourceSecret), &corev1.Secret{}))
}

// Deliver a real status-subresource update deterministically on the second
// polling read, after a transient first-read failure, without background sleeps.
type sourceWorkAppliedOnReadClient struct {
	client.Client
	t     *testing.T
	reads int
}

func (c *sourceWorkAppliedOnReadClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object,
	opts ...client.GetOption,
) error {
	if _, ok := obj.(*workv1.ManifestWork); ok {
		c.reads++
		if c.reads == 1 {
			return apierrors.NewServerTimeout(schema.GroupResource{Group: workv1.GroupVersion.Group, Resource: "manifestworks"}, "get", 1)
		}
		if c.reads == 2 {
			mc := &clusterv1.ManagedCluster{}
			require.NoError(c.t, c.Client.Get(ctx, client.ObjectKey{Name: key.Namespace}, mc))
			require.True(c.t, mc.Spec.HubAcceptsClient, "admission must still be open when Applied is delivered")
			work := &workv1.ManifestWork{}
			require.NoError(c.t, c.Client.Get(ctx, key, work))
			work.Status.Conditions[0].ObservedGeneration = work.Generation
			require.NoError(c.t, c.Client.Status().Update(ctx, work))
		}
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func TestMigrationSourceRegisteringWaitsForStatus(t *testing.T) {
	event := sourceWorkEvent("cluster1")
	work := migrationSourceWorkFixture(t, "cluster1")
	require.NoError(t, prepareSourceWork(work, event, event.BootstrapSecret))
	work.Generation = 4 // Applied is still for generation 3.
	c := &sourceWorkAppliedOnReadClient{Client: sourceWorkClient(t, work, event.BootstrapSecret, sourceMC("cluster1", true)), t: t}
	syncer := &MigrationSourceSyncer{client: c}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, syncer.registering(ctx, event))
	require.Equal(t, 2, c.reads)
	mc := &clusterv1.ManagedCluster{}
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: "cluster1"}, mc))
	require.False(t, mc.Spec.HubAcceptsClient)
}
