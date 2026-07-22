// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// localClusterTestScheme returns a runtime.Scheme with clusterv1 types registered for tests.
func localClusterTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clusterv1.Install(scheme)
	return scheme
}

// TestResolveLocalClusterManagedClusterName verifies local-cluster ManagedCluster name resolution.
func TestResolveLocalClusterManagedClusterName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		clusters    []clusterv1.ManagedCluster
		expected    string
		expectError bool
	}{
		{
			name:     "no local cluster MC falls back to local-cluster",
			expected: constants.LocalClusterName,
		},
		{
			name: "returns actual MC name when labeled local-cluster",
			clusters: []clusterv1.ManagedCluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "acm-local-cluster",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expected: "acm-local-cluster",
		},
		{
			name: "ignores MC without local-cluster label",
			clusters: []clusterv1.ManagedCluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "regional-hub-1",
					},
				},
			},
			expected: constants.LocalClusterName,
		},
		{
			name: "multiple local clusters is an error",
			clusters: []clusterv1.ManagedCluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-a",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-b",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			objs := make([]client.Object, len(tt.clusters))
			for i := range tt.clusters {
				objs[i] = &tt.clusters[i]
			}

			c := fake.NewClientBuilder().WithScheme(localClusterTestScheme(t)).WithObjects(objs...).Build()
			name, err := ResolveLocalClusterManagedClusterName(context.Background(), c)
			if tt.expectError {
				require.Error(t, err, "ResolveLocalClusterManagedClusterName(%q) should return error", tt.name)
				assert.Contains(t, err.Error(), "expected at most one",
					"ResolveLocalClusterManagedClusterName(%q) should not include cluster names in error", tt.name)
				return
			}

			require.NoError(t, err, "ResolveLocalClusterManagedClusterName(%q) should not return error", tt.name)
			assert.Equal(t, tt.expected, name, "ResolveLocalClusterManagedClusterName(%q) returned unexpected name", tt.name)
		})
	}
}
