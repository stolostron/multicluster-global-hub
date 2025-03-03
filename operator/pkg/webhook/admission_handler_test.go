// Copyright (c) 2022 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package webhook

import (
	"context"
	"testing"

	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

func Test_isInHostedCluster(t *testing.T) {
	tests := []struct {
		name    string
		want    bool
		cluster *clusterv1.ManagedCluster
	}{
		{
			name: "no annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-not",
				},
			},
			want: true,
		},
		{
			name: "has annotation, but not hosted",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						"k": "v",
					},
				},
			},
			want: false,
		},
		{
			name: "has hosted annotation",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						constants.AnnotationClusterDeployMode:         constants.ClusterDeployModeHosted,
						constants.AnnotationClusterHostingClusterName: constants.LocalClusterName,
					},
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		ctx := context.Background()
		client := fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects(tt.cluster).Build()
		t.Run(tt.name, func(t *testing.T) {
			adh := admissionHandler{
				localClusterName: constants.LocalClusterName,
			}
			if got, err := adh.isInHostedCluster(ctx, client, "test"); got != tt.want {
				t.Errorf("isInHostedCluster() = %v, want %v", got, tt.want)
				t.Errorf("err:%v", err)
			}
		})
	}
}
