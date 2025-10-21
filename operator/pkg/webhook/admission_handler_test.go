// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"encoding/json"
	"testing"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func Test_setHostedAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		cluster *clusterv1.ManagedCluster
		want    bool
	}{
		{
			name: "no annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
			},
			want: true,
		},
		{
			name: "has other annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						"a": "b",
					},
				},
			},
			want: true,
		},
		{
			name: "has false annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode:         "b",
						constants.AnnotationClusterHostingClusterName: constants.LocalClusterName,
					},
				},
			},
			want: true,
		},
		{
			name: "has hosted annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode:         constants.ClusterDeployModeHosted,
						constants.AnnotationClusterHostingClusterName: constants.LocalClusterName,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adh := admissionHandler{
				localClusterName: constants.LocalClusterName,
			}
			if got := adh.setHostedAnnotations(tt.cluster); got != tt.want {
				t.Errorf("name:%v, setHostedAnnotations() = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func Test_disableAddons(t *testing.T) {
	tests := []struct {
		name                  string
		klusterletaddonconfig *addonv1.KlusterletAddonConfig
		want                  bool
	}{
		{
			name: "enable all addons",
			klusterletaddonconfig: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			want: true,
		},
		{
			name: "enable some addons",
			klusterletaddonconfig: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			want: true,
		},
		{
			name: "disable all addons",
			klusterletaddonconfig: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := disableAddons(tt.klusterletaddonconfig); got != tt.want {
				t.Errorf("disableAddons() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAdmissionHandler_handleManagedCluster(t *testing.T) {
	scheme := config.GetRuntimeScheme()

	tests := []struct {
		name                 string
		cluster              *clusterv1.ManagedCluster
		existingKAC          *addonv1.KlusterletAddonConfig
		localCluster         *clusterv1.ManagedCluster
		expectedResponseType string
		expectedAllowed      bool
	}{
		{
			name: "local cluster - should be allowed",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "already imported cluster - should be allowed",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "imported-cluster",
				},
				Status: clusterv1.ManagedClusterStatus{
					Conditions: []metav1.Condition{
						{
							Type:   constants.ManagedClusterImportSucceeded,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "cluster without deploy mode label - should be allowed",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-label-cluster",
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "cluster with default deploy mode - should be allowed",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-cluster",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeDefault,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "hosted cluster without KAC - should set annotations only",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hosted-cluster",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "patch",
			expectedAllowed:      true,
		},
		{
			name: "hosted cluster with existing KAC - should update KAC and set annotations",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hosted-cluster-with-kac",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			existingKAC: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hosted-cluster-with-kac",
					Namespace: "hosted-cluster-with-kac",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "patch",
			expectedAllowed:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{tt.localCluster}
			if tt.existingKAC != nil {
				objs = append(objs, tt.existingKAC)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			admissionHandler := &admissionHandler{
				client:           client,
				localClusterName: "local-cluster",
				decoder:          admission.NewDecoder(scheme),
			}

			clusterJSON, _ := json.Marshal(tt.cluster)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: clusterJSON},
				},
			}

			ctx := context.Background()
			resp := admissionHandler.handleManagedCluster(ctx, req)

			if tt.expectedAllowed {
				if !resp.Allowed {
					t.Errorf("expected response to be allowed, but got denied: %v", resp.Result)
				}
			} else {
				if resp.Allowed {
					t.Errorf("expected response to be denied, but got allowed")
				}
			}

			if tt.expectedResponseType == "patch" {
				if len(resp.Patches) == 0 {
					t.Errorf("expected patch response but got no patches")
				}
			}

			// Verify KAC was updated if it existed
			if tt.existingKAC != nil {
				updatedKAC := &addonv1.KlusterletAddonConfig{}
				err := client.Get(ctx, types.NamespacedName{Name: tt.existingKAC.Name, Namespace: tt.existingKAC.Namespace}, updatedKAC)
				if err != nil {
					t.Errorf("failed to get updated KAC: %v", err)
				} else {
					// Verify addons are disabled
					if updatedKAC.Spec.ApplicationManagerConfig.Enabled {
						t.Errorf("expected ApplicationManagerConfig to be disabled")
					}
					if updatedKAC.Spec.PolicyController.Enabled {
						t.Errorf("expected PolicyController to be disabled")
					}
					if updatedKAC.Spec.CertPolicyControllerConfig.Enabled {
						t.Errorf("expected CertPolicyControllerConfig to be disabled")
					}
				}
			}
		})
	}
}

func TestAdmissionHandler_handleManagedCluster_KACUpdate(t *testing.T) {
	scheme := config.GetRuntimeScheme()

	tests := []struct {
		name               string
		initialKAC         *addonv1.KlusterletAddonConfig
		cluster            *clusterv1.ManagedCluster
		localCluster       *clusterv1.ManagedCluster
		expectKACDisabled  bool
		expectKACUnchanged bool
	}{
		{
			name: "hosted cluster with enabled KAC - should disable all addons",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-hosted-cluster",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			initialKAC: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hosted-cluster",
					Namespace: "test-hosted-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectKACDisabled: true,
		},
		{
			name: "hosted cluster with already disabled KAC - should remain unchanged",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-hosted-cluster-disabled",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			initialKAC: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hosted-cluster-disabled",
					Namespace: "test-hosted-cluster-disabled",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectKACUnchanged: true,
		},
		{
			name: "default mode cluster with enabled KAC - should remain unchanged",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-default-cluster",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeDefault,
					},
				},
			},
			initialKAC: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-default-cluster",
					Namespace: "test-default-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectKACUnchanged: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{tt.localCluster, tt.initialKAC}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			admissionHandler := &admissionHandler{
				client:           client,
				localClusterName: "local-cluster",
				decoder:          admission.NewDecoder(scheme),
			}

			clusterJSON, _ := json.Marshal(tt.cluster)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: clusterJSON},
				},
			}

			ctx := context.Background()

			// Get initial KAC state
			initialKAC := &addonv1.KlusterletAddonConfig{}
			err := client.Get(ctx, types.NamespacedName{Name: tt.initialKAC.Name, Namespace: tt.initialKAC.Namespace}, initialKAC)
			if err != nil {
				t.Fatalf("failed to get initial KAC: %v", err)
			}

			// Process the ManagedCluster
			resp := admissionHandler.handleManagedCluster(ctx, req)
			if !resp.Allowed {
				t.Errorf("expected response to be allowed, but got denied: %v", resp.Result)
			}

			// Verify KAC state after processing
			updatedKAC := &addonv1.KlusterletAddonConfig{}
			err = client.Get(ctx, types.NamespacedName{Name: tt.initialKAC.Name, Namespace: tt.initialKAC.Namespace}, updatedKAC)
			if err != nil {
				t.Fatalf("failed to get updated KAC: %v", err)
			}

			if tt.expectKACDisabled {
				// Verify all addons are disabled
				if updatedKAC.Spec.ApplicationManagerConfig.Enabled {
					t.Errorf("expected ApplicationManagerConfig to be disabled, but it's enabled")
				}
				if updatedKAC.Spec.PolicyController.Enabled {
					t.Errorf("expected PolicyController to be disabled, but it's enabled")
				}
				if updatedKAC.Spec.CertPolicyControllerConfig.Enabled {
					t.Errorf("expected CertPolicyControllerConfig to be disabled, but it's enabled")
				}
			}

			if tt.expectKACUnchanged {
				// Verify KAC state is unchanged
				if updatedKAC.Spec.ApplicationManagerConfig.Enabled != initialKAC.Spec.ApplicationManagerConfig.Enabled {
					t.Errorf("ApplicationManagerConfig state changed unexpectedly: initial=%v, updated=%v",
						initialKAC.Spec.ApplicationManagerConfig.Enabled, updatedKAC.Spec.ApplicationManagerConfig.Enabled)
				}
				if updatedKAC.Spec.PolicyController.Enabled != initialKAC.Spec.PolicyController.Enabled {
					t.Errorf("PolicyController state changed unexpectedly: initial=%v, updated=%v",
						initialKAC.Spec.PolicyController.Enabled, updatedKAC.Spec.PolicyController.Enabled)
				}
				if updatedKAC.Spec.CertPolicyControllerConfig.Enabled != initialKAC.Spec.CertPolicyControllerConfig.Enabled {
					t.Errorf("CertPolicyControllerConfig state changed unexpectedly: initial=%v, updated=%v",
						initialKAC.Spec.CertPolicyControllerConfig.Enabled, updatedKAC.Spec.CertPolicyControllerConfig.Enabled)
				}
			}
		})
	}
}

func TestAdmissionHandler_handleKlusterletAddonConfig(t *testing.T) {
	scheme := config.GetRuntimeScheme()

	tests := []struct {
		name                 string
		kac                  *addonv1.KlusterletAddonConfig
		existingMC           *clusterv1.ManagedCluster
		localCluster         *clusterv1.ManagedCluster
		expectedResponseType string
		expectedAllowed      bool
	}{
		{
			name: "KAC without corresponding ManagedCluster - should be allowed",
			kac: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nonexistent-cluster",
					Namespace: "nonexistent-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "KAC with non-hosted ManagedCluster - should be allowed",
			kac: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "normal-cluster",
					Namespace: "normal-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			existingMC: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "normal-cluster",
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
		{
			name: "KAC with hosted ManagedCluster and enabled addons - should disable addons",
			kac: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hosted-cluster",
					Namespace: "hosted-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			existingMC: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hosted-cluster",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode:         constants.ClusterDeployModeHosted,
						constants.AnnotationClusterHostingClusterName: "local-cluster",
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "patch",
			expectedAllowed:      true,
		},
		{
			name: "KAC with hosted ManagedCluster and already disabled addons - should be allowed",
			kac: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "hosted-cluster-disabled",
					Namespace: "hosted-cluster-disabled",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					PolicyController: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
					CertPolicyControllerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: false,
					},
				},
			},
			existingMC: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hosted-cluster-disabled",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode:         constants.ClusterDeployModeHosted,
						constants.AnnotationClusterHostingClusterName: "local-cluster",
					},
				},
			},
			localCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "local-cluster",
					Labels: map[string]string{
						constants.LocalClusterName: "true",
					},
				},
			},
			expectedResponseType: "allowed",
			expectedAllowed:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []runtime.Object{tt.localCluster}
			if tt.existingMC != nil {
				objs = append(objs, tt.existingMC)
			}
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

			admissionHandler := &admissionHandler{
				client:           client,
				localClusterName: "local-cluster",
				decoder:          admission.NewDecoder(scheme),
			}

			kacJSON, _ := json.Marshal(tt.kac)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: kacJSON},
				},
			}

			ctx := context.Background()
			resp := admissionHandler.handleKlusterletAddonConfig(ctx, req)

			if tt.expectedAllowed {
				if !resp.Allowed {
					t.Errorf("expected response to be allowed, but got denied: %v", resp.Result)
				}
			} else {
				if resp.Allowed {
					t.Errorf("expected response to be denied, but got allowed")
				}
			}

			if tt.expectedResponseType == "patch" {
				if len(resp.Patches) == 0 {
					t.Errorf("expected patch response but got no patches")
				}
			}
		})
	}
}

func TestAdmissionHandler_Handle_CrossCheck(t *testing.T) {
	scheme := config.GetRuntimeScheme()

	tests := []struct {
		name            string
		kind            string
		object          runtime.Object
		existingObjects []runtime.Object
		expectedAllowed bool
		expectedPatches int
	}{
		{
			name: "ManagedCluster without KlusterletAddonConfig - should be allowed",
			kind: "ManagedCluster",
			object: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
					Labels: map[string]string{
						constants.GHDeployModeLabelKey: constants.GHDeployModeHosted,
					},
				},
			},
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expectedAllowed: true,
			expectedPatches: 1,
		},
		{
			name: "KlusterletAddonConfig without ManagedCluster - should be allowed",
			kind: "KlusterletAddonConfig",
			object: &addonv1.KlusterletAddonConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-cluster",
				},
				Spec: addonv1.KlusterletAddonConfigSpec{
					ApplicationManagerConfig: addonv1.KlusterletAddonAgentConfigSpec{
						Enabled: true,
					},
				},
			},
			existingObjects: []runtime.Object{
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "local-cluster",
						Labels: map[string]string{
							constants.LocalClusterName: "true",
						},
					},
				},
			},
			expectedAllowed: true,
			expectedPatches: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.existingObjects...).Build()

			admissionHandler := &admissionHandler{
				client:           client,
				localClusterName: "local-cluster",
				decoder:          admission.NewDecoder(scheme),
			}

			objJSON, _ := json.Marshal(tt.object)
			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind:   metav1.GroupVersionKind{Kind: tt.kind},
					Object: runtime.RawExtension{Raw: objJSON},
				},
			}

			ctx := context.Background()
			resp := admissionHandler.Handle(ctx, req)

			if tt.expectedAllowed != resp.Allowed {
				t.Errorf("expected allowed=%v, got allowed=%v, result=%v", tt.expectedAllowed, resp.Allowed, resp.Result)
			}

			if len(resp.Patches) != tt.expectedPatches {
				t.Errorf("expected %d patches, got %d patches", tt.expectedPatches, len(resp.Patches))
			}
		})
	}
}
