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

package addon

import (
	"context"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
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
									Name:      "non-local-cluster",
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
									Name:      "non-local-cluster",
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
									Name:      "non-local-cluster",
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
									Name:      "non-local-cluster",
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
				t.Errorf("expectCma() = %v, want %v", tt.expectCma.Spec, tt.cma.Spec)
			}
		})
	}
}

func TestPruneReconciler_hasManagedHub(t *testing.T) {
	tests := []struct {
		name    string
		cmas    []runtime.Object
		want    bool
		wantErr bool
	}{
		{
			name:    "no mca",
			cmas:    []runtime.Object{},
			want:    false,
			wantErr: false,
		},
		{
			name: "has mca",
			cmas: []runtime.Object{
				&addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      constants.GHManagedClusterAddonName,
						Namespace: "mh1",
					},
				},
			},
			want:    true,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addonv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.cmas...).Build()
			hac := HostedAgentController{
				c: fakeClient,
			}
			ctx := context.Background()
			got, err := hac.hasManagedHub(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("PruneReconciler.hasManagedHub() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PruneReconciler.hasManagedHub() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPruneReconciler_revertClusterManagementAddon(t *testing.T) {
	tests := []struct {
		name    string
		cmas    []runtime.Object
		wantErr bool
	}{
		{
			name:    "no cma",
			cmas:    []runtime.Object{},
			wantErr: false,
		},
		{
			name: "cma do not have placements",
			cmas: []runtime.Object{
				&addonv1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
				},
			},
			wantErr: false,
		},
		{
			name: "cma do not have target placements",
			cmas: []runtime.Object{
				&addonv1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: addonv1alpha1.ClusterManagementAddOnSpec{
						InstallStrategy: addonv1alpha1.InstallStrategy{
							Placements: []addonv1alpha1.PlacementStrategy{
								{
									PlacementRef: addonv1alpha1.PlacementRef{
										Namespace: constants.GHDefaultNamespace,
										Name:      "global",
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
			},
			wantErr: false,
		},
		{
			name: "cma have target placements",
			cmas: []runtime.Object{
				&addonv1alpha1.ClusterManagementAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "work-manager",
						Namespace: utils.GetDefaultNamespace(),
					},
					Spec: addonv1alpha1.ClusterManagementAddOnSpec{
						InstallStrategy: addonv1alpha1.InstallStrategy{
							Placements: []addonv1alpha1.PlacementStrategy{
								{
									PlacementRef: addonv1alpha1.PlacementRef{
										Namespace: constants.GHDefaultNamespace,
										Name:      "non-local-cluster",
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
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addonv1alpha1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.cmas...).Build()
			hac := HostedAgentController{
				c: fakeClient,
			}
			ctx := context.Background()
			if err := hac.revertClusterManagementAddon(ctx); (err != nil) != tt.wantErr {
				t.Errorf("PruneReconciler.revertClusterManagementAddon() error = %v, wantErr %v", err, tt.wantErr)
			}
			cmaList := &addonv1alpha1.ClusterManagementAddOnList{}

			err := hac.c.List(ctx, cmaList)
			if err != nil {
				t.Errorf("Failed to list cma:%v", err)
			}
			for _, cma := range cmaList.Items {
				if !config.HostedAddonList.Has(cma.Name) {
					continue
				}
				for _, pl := range cma.Spec.InstallStrategy.Placements {
					if reflect.DeepEqual(pl.PlacementRef, config.GlobalHubHostedAddonPlacementStrategy.PlacementRef) {
						t.Errorf("Failed to revert cma")
					}
				}
			}
		})
	}
}
