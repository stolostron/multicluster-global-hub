// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
	assert.Equal(t, "global-hub/acm-local-cluster", FormatGlobalHubStandbySubject("acm-local-cluster"),
		"FormatGlobalHubStandbySubject should prefix global-hub local MC names")
	assert.Equal(t, "global-hub/local-cluster", FormatGlobalHubStandbySubject(constants.LocalClusterName),
		"FormatGlobalHubStandbySubject should prefix the default local-cluster name")
	assert.Empty(t, FormatGlobalHubStandbySubject(""),
		"FormatGlobalHubStandbySubject should return empty for empty input")
}

func TestResolveGlobalHubStandbySubject(t *testing.T) {
	t.Parallel()
	name, ok := ResolveGlobalHubStandbySubject("global-hub/acm-local-cluster")
	require.True(t, ok, "ResolveGlobalHubStandbySubject should accept prefixed subjects")
	assert.Equal(t, "acm-local-cluster", name,
		"ResolveGlobalHubStandbySubject should strip the global-hub prefix")

	_, ok = ResolveGlobalHubStandbySubject("acm-local-cluster")
	assert.False(t, ok, "ResolveGlobalHubStandbySubject should reject unprefixed subjects")

	_, ok = ResolveGlobalHubStandbySubject("global-hub/")
	assert.False(t, ok, "ResolveGlobalHubStandbySubject should reject empty names after prefix")
}

func TestMatchesGlobalHubStandbySubject(t *testing.T) {
	t.Parallel()
	assert.True(t, MatchesGlobalHubStandbySubject("acm-local-cluster", "acm-local-cluster"),
		"MatchesGlobalHubStandbySubject should accept legacy unprefixed subjects")
	assert.True(t, MatchesGlobalHubStandbySubject("global-hub/acm-local-cluster", "acm-local-cluster"),
		"MatchesGlobalHubStandbySubject should accept prefixed global-hub subjects")
	assert.False(t, MatchesGlobalHubStandbySubject("global-hub/other-hub", "acm-local-cluster"),
		"MatchesGlobalHubStandbySubject should reject mismatched prefixed subjects")
	assert.False(t, MatchesGlobalHubStandbySubject("other-hub", "acm-local-cluster"),
		"MatchesGlobalHubStandbySubject should reject unrelated subjects")
}

func TestStandbyHubSourceMatches(t *testing.T) {
	t.Parallel()
	assert.True(t, StandbyHubSourceMatches("acm-local-cluster", "acm-local-cluster"),
		"StandbyHubSourceMatches should accept identical unprefixed values")
	assert.True(t, StandbyHubSourceMatches("acm-local-cluster", "global-hub/acm-local-cluster"),
		"StandbyHubSourceMatches should accept unprefixed source with prefixed config")
	assert.True(t, StandbyHubSourceMatches("global-hub/acm-local-cluster", "acm-local-cluster"),
		"StandbyHubSourceMatches should accept prefixed source with unprefixed config")
	assert.False(t, StandbyHubSourceMatches("regional-hub", "global-hub/acm-local-cluster"),
		"StandbyHubSourceMatches should reject unrelated standby sources")
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
						Name:   "local-a",
						Labels: map[string]string{constants.LocalClusterName: "true"},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:   "local-b",
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
			require.NoError(t, clusterv1.Install(scheme),
				"FindStandbyHubTarget test scheme should register clusterv1 types")
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.clusters...).Build()

			got, err := FindStandbyHubTarget(context.Background(), c, tt.cachedLocal)
			if tt.expectedError {
				require.Error(t, err, "FindStandbyHubTarget(%q) should return error", tt.name)
				return
			}
			require.NoError(t, err, "FindStandbyHubTarget(%q) should not return error", tt.name)
			assert.Equal(t, tt.expected, got,
				"FindStandbyHubTarget(%q) should resolve the expected standby target", tt.name)
		})
	}
}
