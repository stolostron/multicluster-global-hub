// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"fmt"
	"testing"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	mchv1 "github.com/stolostron/multiclusterhub-operator/api/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
)

// This is a source-side simulation, not a live migration. Controller rendering,
// work-agent Applied status, ReadOnly handling, and recovery are modeled below.
// No REST config, kubeconfig loading, network client, or live API is used.
func TestLatePauseSimulatedMigration(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(fmt.Sprintf("ReadOnly=%t", readOnly), func(t *testing.T) {
			ctx := context.Background()
			secret, target := latePauseFixture("dummy-spoke", "dummy-target")
			_, original := latePauseFixture("dummy-spoke", "dummy-source")
			original.Generation = 1
			original.Status.Conditions[0].ObservedGeneration = 1
			mch := &mchv1.MultiClusterHub{ObjectMeta: metav1.ObjectMeta{Name: "dummy-mch"}, Status: mchv1.MultiClusterHubStatus{CurrentVersion: "5.0.0"}}
			mc := &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "dummy-spoke"}, Spec: clusterv1.ManagedClusterSpec{HubAcceptsClient: true}, Status: clusterv1.ManagedClusterStatus{Conditions: []metav1.Condition{{Type: clusterv1.ManagedClusterConditionAvailable, Status: metav1.ConditionTrue}}}}
			events := []string{}
			paused := false
			c := fake.NewClientBuilder().WithScheme(configs.GetRuntimeScheme()).
				WithStatusSubresource(&workv1.ManifestWork{}, &clusterv1.ManagedCluster{}).
				WithObjects(mch, original, mc).
				WithInterceptorFuncs(interceptor.Funcs{Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
					cluster, ok := obj.(*clusterv1.ManagedCluster)
					if !ok {
						return c.Update(ctx, obj, opts...)
					}
					previous := &clusterv1.ManagedCluster{}
					if err := c.Get(ctx, client.ObjectKeyFromObject(cluster), previous); err != nil {
						return err
					}
					_, wantPause := cluster.Annotations[apiconstants.DisableAutoImportAnnotation]
					if wantPause && !paused {
						work := &workv1.ManifestWork{}
						if err := c.Get(ctx, client.ObjectKeyFromObject(original), work); err != nil {
							return err
						}
						// Verify at the write boundary, before any simulated locking occurs.
						digest, err := appliedMigrationWork(work, secret, "dummy-migration", "")
						require.NoError(t, err)
						require.Equal(t, digest, cluster.Annotations[migrationAppliedWorkAnnotation])
						require.True(t, cluster.Spec.HubAcceptsClient, "pause must precede admission removal")
						events = append(events, "pause")
					}
					if !cluster.Spec.HubAcceptsClient && previous.Spec.HubAcceptsClient {
						require.True(t, wantPause, "cutover without pause")
						require.NotEmpty(t, cluster.Annotations[migrationAppliedWorkAnnotation])
						events = append(events, "cutover")
					}
					if err := c.Update(ctx, obj, opts...); err != nil {
						return err
					}
					if wantPause && !paused && readOnly {
						work := &workv1.ManifestWork{}
						if err := c.Get(ctx, client.ObjectKeyFromObject(original), work); err != nil {
							return err
						}
						work.Spec.ManifestConfigs = []workv1.ManifestConfigOption{{UpdateStrategy: &workv1.UpdateStrategy{Type: workv1.UpdateStrategyTypeReadOnly}}}
						work.Generation++
						if err := c.Update(ctx, work); err != nil {
							return err
						}
						events = append(events, "readonly")
					}
					if !wantPause && paused {
						// Model normal import reconciliation after removing migration config.
						work := &workv1.ManifestWork{}
						if err := c.Get(ctx, client.ObjectKeyFromObject(original), work); err != nil {
							return err
						}
						work.Spec = original.Spec
						work.Generation++
						if err := c.Update(ctx, work); err != nil {
							return err
						}
						events = append(events, "restore-source-payload")
					}
					paused = wantPause
					return nil
				}}).Build()
			s := &MigrationSourceSyncer{client: c, clusterErrors: map[string]string{}}
			evt := &migration.MigrationSourceBundle{MigrationId: "dummy-migration", ToHub: "dummy-target", ManagedClusters: []string{mc.Name}, BootstrapSecret: secret}
			require.NoError(t, s.initializing(ctx, evt))
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(mc), mc))
			require.NotContains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
			t.Log("initializing: configuration attached; auto-import remains enabled")

			blocked := func() {
				waitCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
				defer cancel()
				require.Error(t, s.registering(waitCtx, evt))
				require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(mc), mc))
				require.True(t, mc.Spec.HubAcceptsClient)
				require.NotContains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
			}
			blocked()
			t.Log("delayed rendering: old source payload cannot authorize cutover")
			work := &workv1.ManifestWork{}
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(original), work))
			work.Spec = target.Spec
			work.Generation = 2
			require.NoError(t, c.Update(ctx, work))
			blocked()
			t.Log("target payload rendered but old Applied status: admission stays enabled")
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(original), work))
			work.Status.Conditions = target.Status.Conditions
			require.NoError(t, c.Status().Update(ctx, work))
			require.NoError(t, s.registering(ctx, evt))
			expected := []string{"pause", "cutover"}
			if readOnly {
				expected = []string{"pause", "readonly", "cutover"}
			}
			require.Equal(t, expected, events)
			t.Logf("target generation Applied: verified sequence %v", events)

			restarted := &MigrationSourceSyncer{client: c, clusterErrors: map[string]string{}}
			require.NoError(t, restarted.registering(ctx, evt))
			t.Log("agent restart: cutover retry succeeds using persisted apply evidence")
			// Exercise the actual source rollback method; availability is an explicit
			// fixture assumption, not proof of registration on a real source hub.
			require.NoError(t, restarted.rollbackRegistering(ctx, evt))
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(mc), mc))
			require.True(t, mc.Spec.HubAcceptsClient)
			require.NotContains(t, mc.Annotations, apiconstants.DisableAutoImportAnnotation)
			require.NotContains(t, mc.Annotations, migrationAppliedWorkAnnotation)
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(original), work))
			require.Equal(t, original.Spec, work.Spec)
			t.Log("rollback: admission restored, pause/evidence removed, modeled source payload restored")
		})
	}
}
