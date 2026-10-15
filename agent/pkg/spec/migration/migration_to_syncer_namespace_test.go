package migration

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
)

func TestValidateTargetNamespaces(t *testing.T) {
	for _, tc := range []struct {
		name      string
		namespace *corev1.Namespace
		wantError bool
	}{
		{name: "missing namespace is allowed"},
		{
			name: "active namespace is allowed",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
			},
		},
		{
			name: "terminating phase is rejected",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster1"},
				Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
			},
			wantError: true,
		},
		{
			name: "deletion timestamp is rejected even while phase is active",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1", DeletionTimestamp: &metav1.Time{Time: metav1.Now().Time},
					Finalizers: []string{"test.example.com/hold"},
				},
				Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
			},
			wantError: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			builder := fake.NewClientBuilder().WithScheme(configs.GetRuntimeScheme())
			if tc.namespace != nil {
				builder.WithObjects(tc.namespace)
			}
			c := builder.Build()
			syncer := NewMigrationTargetSyncer(c, nil, &configs.AgentConfig{LeafHubName: "target"})
			clusterErrors := map[string]string{}
			err := syncer.validateManagedClusters(ctx, []string{"cluster1"}, clusterErrors)
			if tc.wantError {
				require.Error(t, err)
				require.Contains(t, clusterErrors["cluster1"], "target namespace cluster1 is terminating")
				require.Contains(t, clusterErrors["cluster1"], "before retrying migration")

				// The validating stage must fail before recording initialization state.
				err = syncer.validating(ctx, &migration.MigrationTargetBundle{
					MigrationId: "test-migration", ManagedClusters: []string{"cluster1"},
				}, map[string]string{})
				require.Error(t, err)
				localMigrations := &migrationv1alpha1.ManagedClusterMigrationList{}
				require.NoError(t, c.List(ctx, localMigrations))
				require.Empty(t, localMigrations.Items)

				// Recheck at deployment to cover deletion starting after validation.
				require.ErrorContains(t, syncer.ensureNamespace(ctx, "cluster1"), "is terminating")
			} else {
				require.NoError(t, err)
				require.Empty(t, clusterErrors)
				require.NoError(t, syncer.ensureNamespace(ctx, "cluster1"))
			}
		})
	}
}

func TestValidateTargetNamespacesAggregatesErrors(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(configs.GetRuntimeScheme()).WithObjects(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "terminating"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
		},
		&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "existing"}},
	).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object,
			opts ...client.GetOption,
		) error {
			if _, ok := obj.(*corev1.Namespace); ok && key.Name == "forbidden" {
				return apierrors.NewForbidden(corev1.Resource("namespaces"), key.Name, fmt.Errorf("access denied"))
			}
			return c.Get(ctx, key, obj, opts...)
		},
	}).Build()
	syncer := NewMigrationTargetSyncer(c, nil, &configs.AgentConfig{LeafHubName: "target"})
	clusterErrors := map[string]string{}
	err := syncer.validateManagedClusters(ctx, []string{"terminating", "existing", "forbidden", "healthy"}, clusterErrors)
	require.ErrorContains(t, err, "3 clusters validation failed")
	require.Len(t, clusterErrors, 3)
	require.Contains(t, clusterErrors["terminating"], "is terminating")
	require.Contains(t, clusterErrors["existing"], "already exists")
	require.Contains(t, clusterErrors["forbidden"], "failed to get target namespace forbidden")
	require.NotContains(t, clusterErrors, "healthy")

	// Validation must not create a missing namespace.
	require.True(t, apierrors.IsNotFound(c.Get(ctx, types.NamespacedName{Name: "healthy"}, &corev1.Namespace{})))
}
