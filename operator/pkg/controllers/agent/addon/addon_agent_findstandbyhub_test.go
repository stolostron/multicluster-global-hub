// Copyright Contributors to the Open Cluster Management project.
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

package addon

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

func addonFindStandbyHubTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, clusterv1.Install(scheme))
	return scheme
}

func TestGlobalHubAddonAgent_findStandbyHub(t *testing.T) {
	t.Parallel()

	const localClusterName = "local-cluster"

	tests := []struct {
		name          string
		clusters      []client.Object
		expectedHub   string
		expectedError bool
		errorContains string
	}{
		{
			name: "standby hub exists",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub2",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expectedHub: "hub2",
		},
		{
			name: "no standby hub - returns local-cluster fallback",
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
			expectedHub: localClusterName,
		},
		{
			name: "no standby hub - resolves labeled local cluster MC name",
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
						Name: "hub1",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleActive,
						},
					},
				},
			},
			expectedHub: "acm-local-cluster",
		},
		{
			name: "multiple standby hubs - returns alphabetically first",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub-z",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hub-a",
						Labels: map[string]string{
							constants.GHHubRoleLabelKey: constants.GHHubRoleStandby,
						},
					},
				},
			},
			expectedHub: "hub-a",
		},
		{
			name: "no standby hub - multiple local clusters is an error",
			clusters: []client.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-a",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-b",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expectedError: true,
			errorContains: "failed to resolve local ManagedCluster name",
		},
		{
			name:          "no clusters - returns local-cluster fallback",
			clusters:      []client.Object{},
			expectedHub:   localClusterName,
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fakeClient := fake.NewClientBuilder().
				WithScheme(addonFindStandbyHubTestScheme(t)).
				WithObjects(tt.clusters...).
				Build()

			agent := &GlobalHubAddonAgent{
				ctx:    context.Background(),
				client: fakeClient,
			}

			hub, err := agent.findStandbyHub()
			if tt.expectedError {
				require.Error(t, err, "findStandbyHub(%q) should return error", tt.name)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains,
						"findStandbyHub(%q) error should contain expected context", tt.name)
				}
				return
			}

			require.NoError(t, err, "findStandbyHub(%q) should not return error", tt.name)
			assert.Equal(t, tt.expectedHub, hub, "findStandbyHub(%q) returned unexpected hub name", tt.name)
		})
	}
}
