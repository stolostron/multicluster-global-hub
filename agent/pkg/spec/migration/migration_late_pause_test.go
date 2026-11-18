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
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func latePauseFixture(cluster, target string) (*corev1.Secret, *workv1.ManifestWork) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: bootstrapSecretNamePrefix + target, Namespace: "multicluster-engine"}, Data: map[string][]byte{"kubeconfig": []byte("test-target")}}
	work := &workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{Name: cluster + "-klusterlet", Namespace: cluster, UID: "work-uid", Generation: 2}}
	objects := []map[string]interface{}{
		{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]interface{}{"name": secret.Name, "namespace": "open-cluster-management-agent"}, "data": map[string]interface{}{"kubeconfig": base64.StdEncoding.EncodeToString(secret.Data["kubeconfig"])}},
		{"apiVersion": "operator.open-cluster-management.io/v1", "kind": "Klusterlet", "metadata": map[string]interface{}{"name": "klusterlet"}, "spec": map[string]interface{}{"registrationConfiguration": map[string]interface{}{"featureGates": []interface{}{map[string]interface{}{"feature": "MultipleHubs", "mode": "Enable"}}, "bootstrapKubeConfigs": map[string]interface{}{"type": "LocalSecrets", "localSecretsConfig": map[string]interface{}{"kubeConfigSecrets": []interface{}{map[string]interface{}{"name": secret.Name}}}}}}},
	}
	for _, obj := range objects {
		raw, _ := json.Marshal(obj)
		work.Spec.Workload.Manifests = append(work.Spec.Workload.Manifests, workv1.Manifest{RawExtension: runtime.RawExtension{Raw: raw}})
	}
	work.Status.Conditions = []metav1.Condition{{Type: workv1.WorkApplied, Status: metav1.ConditionTrue, ObservedGeneration: 2}}
	return secret, work
}

func TestLatePausePayloadGate(t *testing.T) {
	secret, work := latePauseFixture("cluster1", "hub2")
	digest, err := appliedMigrationWork(work, secret, "migration1", "")
	require.NoError(t, err)
	work.Generation++
	_, err = appliedMigrationWork(work, secret, "migration1", "")
	require.ErrorContains(t, err, "current migration generation")
	work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly}}}
	_, err = appliedMigrationWork(work, secret, "migration1", "")
	require.ErrorContains(t, err, "ReadOnly")
	_, err = appliedMigrationWork(work, secret, "migration1", digest)
	require.NoError(t, err, "pause retry survives strategy-only generation change")
	_, err = appliedMigrationWork(work, secret, "different-migration", digest)
	require.Error(t, err)
	work.UID = "recreated-work"
	_, err = appliedMigrationWork(work, secret, "migration1", digest)
	require.Error(t, err)
	secret.Data["kubeconfig"] = []byte("rotated-target")
	_, err = appliedMigrationWork(work, secret, "migration1", digest)
	require.ErrorContains(t, err, "lacks target")
}

func TestLatePauseBatchAndRetry(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.AddToScheme(scheme))
	require.NoError(t, workv1.AddToScheme(scheme))
	secret, work := latePauseFixture("cluster1", "hub2")
	_, bad := latePauseFixture("cluster2", "hub2")
	bad.Status.Conditions = nil
	mc1 := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}, Spec: clusterv1.ManagedClusterSpec{HubAcceptsClient: true}}
	mc2 := mc1.DeepCopy()
	mc2.Name = "cluster2"
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&workv1.ManifestWork{}).WithObjects(secret, work, bad, mc1, mc2).Build()
	s := &MigrationSourceSyncer{client: c}
	evt := &migration.MigrationSourceBundle{MigrationId: "migration1", ToHub: "hub2", ManagedClusters: []string{"cluster1", "cluster2"}}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.Error(t, s.registering(ctx, evt))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc1.Name}, mc1))
	require.True(t, mc1.Spec.HubAcceptsClient)
	require.NotContains(t, mc1.Annotations, apiconstants.DisableAutoImportAnnotation)
	evt.ManagedClusters = []string{"cluster1"}
	require.NoError(t, s.registering(context.Background(), evt))
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Name: mc1.Name}, mc1))
	require.False(t, mc1.Spec.HubAcceptsClient)
	require.Contains(t, mc1.Annotations, apiconstants.DisableAutoImportAnnotation)
	require.Contains(t, mc1.Annotations, migrationAppliedWorkAnnotation)
	// Simulate the existing import controller locking the work after the pause.
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(work), work))
	work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly}}}
	require.NoError(t, c.Update(context.Background(), work))
	require.NoError(t, (&MigrationSourceSyncer{client: c}).registering(context.Background(), evt))
}
