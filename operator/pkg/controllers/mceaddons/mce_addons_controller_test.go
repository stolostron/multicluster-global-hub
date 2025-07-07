/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mceaddons

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestHostedAgentConfig(t *testing.T) {
	tests := []struct {
		name      string
		cma       *addonv1alpha1.ClusterManagementAddOn
		expectCma *addonv1alpha1.ClusterManagementAddOn
		want      bool
	}{
		{
			name: "empty spec",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "c1",
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "c1",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Type: "Manual",
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: constants.GHDefaultNamespace,
									Name:      "non-default-cluster",
								},
								Configs: []addonv1alpha1.AddOnConfig{
									{
										ConfigReferent: addonv1alpha1.ConfigReferent{
											Name:      "global-hub",
											Namespace: constants.GHDefaultNamespace,
										},
										ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
											Group:    "addon.open-cluster-management.io",
											Resource: "addondeploymentconfigs",
										},
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "has config in spec",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name: "work-manager",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Type: "Manual",
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "ns",
									Name:      "pl",
								},
							},
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "c1",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Type: "Manual",
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "ns",
									Name:      "pl",
								},
							},
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: constants.GHDefaultNamespace,
									Name:      "non-default-cluster",
								},
								Configs: []addonv1alpha1.AddOnConfig{
									{
										ConfigReferent: addonv1alpha1.ConfigReferent{
											Name:      "global-hub",
											Namespace: constants.GHDefaultNamespace,
										},
										ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
											Group:    "addon.open-cluster-management.io",
											Resource: "addondeploymentconfigs",
										},
									},
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "has needed config in spec",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "c1",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Type: "Manual",
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: constants.GHDefaultNamespace,
									Name:      "non-default-cluster",
								},
								Configs: []addonv1alpha1.AddOnConfig{
									{
										ConfigReferent: addonv1alpha1.ConfigReferent{
											Name:      "global-hub",
											Namespace: constants.GHDefaultNamespace,
										},
										ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
											Group:    "addon.open-cluster-management.io",
											Resource: "addondeploymentconfigs",
										},
									},
								},
							},
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "c1",
				},
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Type: "Manual",
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: constants.GHDefaultNamespace,
									Name:      "non-default-cluster",
								},
								Configs: []addonv1alpha1.AddOnConfig{
									{
										ConfigReferent: addonv1alpha1.ConfigReferent{
											Name:      "global-hub",
											Namespace: constants.GHDefaultNamespace,
										},
										ConfigGroupResource: addonv1alpha1.ConfigGroupResource{
											Group:    "addon.open-cluster-management.io",
											Resource: "addondeploymentconfigs",
										},
									},
								},
							},
						},
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addAddonConfig(tt.cma)
			if got != tt.want {
				t.Errorf("addAddonConfig() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.expectCma.Spec.InstallStrategy.Placements, tt.cma.Spec.InstallStrategy.Placements) {
				t.Errorf("expectCma() = %v, currentCma() = %v", tt.expectCma.Spec, tt.cma.Spec)
			}
		})
	}
}

func TestRemoveAddonConfig(t *testing.T) {
	tests := []struct {
		name      string
		cma       *addonv1alpha1.ClusterManagementAddOn
		expectCma *addonv1alpha1.ClusterManagementAddOn
		want      bool
	}{
		{
			name: "empty placements",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{},
					},
				},
			},
			want: false,
		},
		{
			name: "no matching placement",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
						},
					},
				},
			},
			want: false,
		},
		{
			name: "single matching placement",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							config.GlobalHubHostedAddonPlacementStrategy,
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{},
			},
			want: true,
		},
		{
			name: "multiple placements, one matching",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
							config.GlobalHubHostedAddonPlacementStrategy,
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
						},
					},
				},
			},
			want: true,
		},
		{
			name: "multiple placements, multiple matching",
			cma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							config.GlobalHubHostedAddonPlacementStrategy,
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
							config.GlobalHubHostedAddonPlacementStrategy,
						},
					},
				},
			},
			expectCma: &addonv1alpha1.ClusterManagementAddOn{
				Spec: addonv1alpha1.ClusterManagementAddOnSpec{
					InstallStrategy: addonv1alpha1.InstallStrategy{
						Placements: []addonv1alpha1.PlacementStrategy{
							{
								PlacementRef: addonv1alpha1.PlacementRef{
									Namespace: "other-ns",
									Name:      "other-pl",
								},
							},
						},
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeAddonConfig(tt.cma)
			if got != tt.want {
				t.Errorf("removeAddonConfig() = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.expectCma.Spec.InstallStrategy.Placements, tt.cma.Spec.InstallStrategy.Placements) {
				t.Errorf("expectCma placements = %v, want %v", tt.cma.Spec.InstallStrategy.Placements, tt.expectCma.Spec.InstallStrategy.Placements)
			}
		})
	}
}
