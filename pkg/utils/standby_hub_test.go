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

func TestFormatGlobalHubStandbySubject(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "global-hub/acm-local-cluster", FormatGlobalHubStandbySubject("acm-local-cluster"))
	assert.Equal(t, "global-hub/local-cluster", FormatGlobalHubStandbySubject(constants.LocalClusterName))
	assert.Empty(t, FormatGlobalHubStandbySubject(""))
}

func TestResolveGlobalHubStandbySubject(t *testing.T) {
	t.Parallel()
	name, ok := ResolveGlobalHubStandbySubject("global-hub/acm-local-cluster")
	require.True(t, ok)
	assert.Equal(t, "acm-local-cluster", name)

	_, ok = ResolveGlobalHubStandbySubject("acm-local-cluster")
	assert.False(t, ok)

	_, ok = ResolveGlobalHubStandbySubject("global-hub/")
	assert.False(t, ok)
}

func TestMatchesGlobalHubStandbySubject(t *testing.T) {
	t.Parallel()
	assert.True(t, MatchesGlobalHubStandbySubject("acm-local-cluster", "acm-local-cluster"))
	assert.True(t, MatchesGlobalHubStandbySubject("global-hub/acm-local-cluster", "acm-local-cluster"))
	assert.False(t, MatchesGlobalHubStandbySubject("global-hub/other-hub", "acm-local-cluster"))
	assert.False(t, MatchesGlobalHubStandbySubject("other-hub", "acm-local-cluster"))
}

func TestStandbyHubSourceMatches(t *testing.T) {
	t.Parallel()
	assert.True(t, StandbyHubSourceMatches("acm-local-cluster", "acm-local-cluster"))
	assert.True(t, StandbyHubSourceMatches("acm-local-cluster", "global-hub/acm-local-cluster"))
	assert.True(t, StandbyHubSourceMatches("global-hub/acm-local-cluster", "acm-local-cluster"))
	assert.False(t, StandbyHubSourceMatches("regional-hub", "global-hub/acm-local-cluster"))
}

func TestFindStandbyHubTarget(t *testing.T) {
	t.Parallel()

	const localClusterName = constants.LocalClusterName

	tests := []struct {
		name          string
		clusters      []client.Object
		cachedLocal   string
		expected      string
		expectedError bool
	}{
		{
			name: "labeled local MC uses prefixed global-hub subject",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "acm-local-cluster",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: localClusterName,
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expected: "global-hub/acm-local-cluster",
		},
		{
			name: "no standby MC falls back to prefixed local-cluster",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
			},
			expected: "global-hub/local-cluster",
		},
		{
			name: "operator cache used when resolver returns default name",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
			},
			cachedLocal: "acm-local-cluster",
			expected:    "global-hub/acm-local-cluster",
		},
		{
			name: "regional standby MC without local-cluster name is unprefixed",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub2",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expected: "hub2",
		},
		{
			name: "multiple local clusters is an error",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-a",
						Labels: map[string]string{constants.LocalClusterName: "true"},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-b",
						Labels: map[string]string{constants.LocalClusterName: "true"},
					},
				},
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			require.NoError(t, clusterv1.Install(scheme))
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.clusters...).Build()

			got, err := FindStandbyHubTarget(context.Background(), c, tt.cachedLocal)
			if tt.expectedError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}
