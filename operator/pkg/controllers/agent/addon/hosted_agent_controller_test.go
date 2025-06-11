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

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestIfHostedLabelChange(t *testing.T) {
	tests := []struct {
		name        string
		newLabels   map[string]string
		oldLabels   map[string]string
		wantChanged bool
	}{
		{
			name:        "nil maps",
			newLabels:   nil,
			oldLabels:   nil,
			wantChanged: false,
		},
		{
			name:        "empty maps",
			newLabels:   map[string]string{},
			oldLabels:   map[string]string{},
			wantChanged: false,
		},
		{
			name:        "same label value",
			newLabels:   map[string]string{constants.GHDeployModeLabelKey: "hosted"},
			oldLabels:   map[string]string{constants.GHDeployModeLabelKey: "hosted"},
			wantChanged: false,
		},
		{
			name:        "different label value",
			newLabels:   map[string]string{constants.GHDeployModeLabelKey: "hosted"},
			oldLabels:   map[string]string{constants.GHDeployModeLabelKey: "non-hosted"},
			wantChanged: true,
		},
		{
			name:        "label added",
			newLabels:   map[string]string{constants.GHDeployModeLabelKey: "hosted"},
			oldLabels:   map[string]string{},
			wantChanged: true,
		},
		{
			name:        "label removed",
			newLabels:   map[string]string{},
			oldLabels:   map[string]string{constants.GHDeployModeLabelKey: "hosted"},
			wantChanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IfHostedLabelChange(tt.newLabels, tt.oldLabels); got != tt.wantChanged {
				t.Errorf("IfHostedLabelChange() = %v, want %v", got, tt.wantChanged)
			}
		})
	}
}

func TestRemoveAddonConfig(t *testing.T) {
	tests := []struct {
		name     string
		mca      *addonv1alpha1.ManagedClusterAddOn
		expected bool
	}{
		{
			name: "empty config list",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{},
				},
			},
			expected: false,
		},
		{
			name: "config not found",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						{
							ConfigReferent: addonv1alpha1.ConfigReferent{
								Name:      "other-config",
								Namespace: "test",
							},
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "config found and removed",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						GlobalHubHostedAddonConfig,
					},
				},
			},
			expected: true,
		},
		{
			name: "config found among others",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						{
							ConfigReferent: addonv1alpha1.ConfigReferent{
								Name:      "other-config",
								Namespace: "test",
							},
						},
						GlobalHubHostedAddonConfig,
						{
							ConfigReferent: addonv1alpha1.ConfigReferent{
								Name:      "another-config",
								Namespace: "test",
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeAddonConfig(tt.mca)
			if result != tt.expected {
				t.Errorf("removeAddonConfig() = %v, want %v", result, tt.expected)
			}

			if tt.expected {
				// Verify config was actually removed
				for _, config := range tt.mca.Spec.Configs {
					if reflect.DeepEqual(config, GlobalHubHostedAddonConfig) {
						t.Error("GlobalHubHostedAddonConfig still present after removal")
					}
				}
			}
		})
	}
}

func TestAddAddonConfig(t *testing.T) {
	tests := []struct {
		name     string
		mca      *addonv1alpha1.ManagedClusterAddOn
		expected bool
	}{
		{
			name: "empty config list",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{},
				},
			},
			expected: true,
		},
		{
			name: "config already exists",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						GlobalHubHostedAddonConfig,
					},
				},
			},
			expected: false,
		},
		{
			name: "config added to existing configs",
			mca: &addonv1alpha1.ManagedClusterAddOn{
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						{
							ConfigReferent: addonv1alpha1.ConfigReferent{
								Name:      "other-config",
								Namespace: "test",
							},
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			initialLen := len(tt.mca.Spec.Configs)
			result := addAddonConfig(tt.mca)
			if result != tt.expected {
				t.Errorf("addAddonConfig() = %v, want %v", result, tt.expected)
			}

			if tt.expected {
				// Verify config was actually added
				if len(tt.mca.Spec.Configs) != initialLen+1 {
					t.Error("Config list length did not increase by 1")
				}
				found := false
				for _, config := range tt.mca.Spec.Configs {
					if reflect.DeepEqual(config, GlobalHubHostedAddonConfig) {
						found = true
						break
					}
				}
				if !found {
					t.Error("GlobalHubHostedAddonConfig not found after addition")
				}
			}
		})
	}
}

func TestReconcile(t *testing.T) {
	tests := []struct {
		name      string
		mca       addonv1alpha1.ManagedClusterAddOn
		mc        clusterv1.ManagedCluster
		expectMca addonv1alpha1.ManagedClusterAddOn
		wantErr   bool
	}{
		{
			name: "mca not found",
			mc: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mc1",
				},
			},
			wantErr: false,
		},
		{
			name: "mc not found",
			mca: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "mc1",
				},
			},
			wantErr: true,
		},
		{
			name: "mc not hosted",
			mca: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "mc1",
				},
			},
			mc: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mc1",
				},
			},
			expectMca: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "mc1",
				},
			},
			wantErr: false,
		},
		{
			name: "mc hosted",
			mca: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "mc1",
				},
			},
			mc: clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "mc1",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			expectMca: addonv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "work-manager",
					Namespace: "mc1",
				},
				Spec: addonv1alpha1.ManagedClusterAddOnSpec{
					Configs: []addonv1alpha1.AddOnConfig{
						GlobalHubHostedAddonConfig,
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = addonv1alpha1.AddToScheme(scheme)
			_ = clusterv1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().WithScheme(scheme)

			fakeClient = fakeClient.WithObjects(&tt.mca).WithObjects(&tt.mc)

			c := &HostedAgentController{
				c: fakeClient.Build(),
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "work-manager",
					Namespace: "mc1",
				},
			}

			_, err := c.Reconcile(context.Background(), req)
			if (err != nil) != tt.wantErr {
				t.Errorf("name: %v, Reconcile() error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			mca := &addonv1alpha1.ManagedClusterAddOn{}
			err = c.c.Get(context.Background(), req.NamespacedName, mca)
			if err == nil {
				if !reflect.DeepEqual(mca.Spec.Configs, tt.expectMca.Spec.Configs) {
					t.Errorf("tt.name: %v ,Reconcile() ManagedClusterAddOn configs = %v, want %v", tt.name, mca.Spec.Configs, tt.expectMca.Spec.Configs)
				}
			}
			if err != nil && !errors.IsNotFound(err) {
				t.Errorf("Reconcile() error getting ManagedClusterAddOn: %v", err)
			}
		})
	}
}
