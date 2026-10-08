// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// This test uses real apiserver spec-generation and status-subresource behavior.
// The minimal structural schemas below are not a substitute for deployed work
// agent testing; status acknowledgements are delivered explicitly by the test.
func TestMigrationSourceWorkEnvtest(t *testing.T) {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("envtest assets not configured; run make unit-tests-agent")
	}
	preserve := true
	newCRD := func(group, plural, singular, kind string, scope apiextensionsv1.ResourceScope) *apiextensionsv1.CustomResourceDefinition {
		return &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: plural + "." + group},
			Spec: apiextensionsv1.CustomResourceDefinitionSpec{
				Group: group, Scope: scope,
				Names: apiextensionsv1.CustomResourceDefinitionNames{Plural: plural, Singular: singular, Kind: kind, ListKind: kind + "List"},
				Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
					Name: "v1", Served: true, Storage: true,
					Subresources: &apiextensionsv1.CustomResourceSubresources{Status: &apiextensionsv1.CustomResourceSubresourceStatus{}},
					Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type: "object", Properties: map[string]apiextensionsv1.JSONSchemaProps{
							"spec":   {Type: "object", XPreserveUnknownFields: &preserve},
							"status": {Type: "object", XPreserveUnknownFields: &preserve},
						},
					}},
				}},
			},
		}
	}
	testEnv := &envtest.Environment{CRDs: []*apiextensionsv1.CustomResourceDefinition{
		newCRD(workv1.GroupVersion.Group, "manifestworks", "manifestwork", "ManifestWork", apiextensionsv1.NamespaceScoped),
		newCRD(clusterv1.GroupVersion.Group, "managedclusters", "managedcluster", "ManagedCluster", apiextensionsv1.ClusterScoped),
	}}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, clusterv1.Install(scheme))
	require.NoError(t, workv1.Install(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	ctx := context.Background()
	for _, name := range []string{"cluster1", "multicluster-engine"} {
		require.NoError(t, c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}))
	}
	mc := sourceMC("cluster1", true)
	mc.Status = clusterv1.ManagedClusterStatus{}
	require.NoError(t, c.Create(ctx, mc))
	event := sourceWorkEvent("cluster1")
	require.NoError(t, c.Create(ctx, event.BootstrapSecret))
	work := migrationSourceWorkFixture(t, "cluster1")
	work.OwnerReferences[0].UID = mc.UID
	work.Generation = 0
	work.Status = workv1.ManifestWorkStatus{}
	require.NoError(t, c.Create(ctx, work))
	require.NoError(t, c.Get(ctx, sourceWorkKey("cluster1"), work))
	require.Equal(t, int64(1), work.Generation)
	work.Status.Conditions = []metav1.Condition{{
		Type: workv1.WorkApplied, Status: metav1.ConditionTrue,
		ObservedGeneration: work.Generation, Reason: "AppliedManifestWorkComplete", LastTransitionTime: metav1.Now(),
	}}
	require.NoError(t, c.Status().Update(ctx, work))
	// Supply REST config to exercise the uncached source-work API client too.
	syncer := &MigrationSourceSyncer{client: c, restConfig: cfg}
	require.NoError(t, syncer.ensureMigrationSourceWork(ctx, "cluster1", event, event.BootstrapSecret))
	require.NoError(t, c.Get(ctx, sourceWorkKey("cluster1"), work))
	require.Equal(t, int64(2), work.Generation)
	require.Equal(t, int64(1), work.Status.Conditions[0].ObservedGeneration)
	require.Error(t, syncer.checkMigrationSourceWork(ctx, "cluster1", event, event.BootstrapSecret))
	deadline, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	require.Error(t, syncer.registering(deadline, event))
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: "cluster1"}, mc))
	require.True(t, mc.Spec.HubAcceptsClient)
	work.Status.Conditions[0].ObservedGeneration = work.Generation
	require.NoError(t, c.Status().Update(ctx, work))
	require.NoError(t, syncer.checkMigrationSourceWork(ctx, "cluster1", event, event.BootstrapSecret))
	require.NoError(t, syncer.registering(ctx, event))
	require.NoError(t, c.Get(ctx, client.ObjectKey{Name: "cluster1"}, mc))
	require.False(t, mc.Spec.HubAcceptsClient)
	require.ErrorContains(t, syncer.restoreMigrationSourceWork(ctx, "cluster1", event), "source admission is disabled")
}
