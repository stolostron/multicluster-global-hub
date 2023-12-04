package addon

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
)

func TestHohAgentAddon_getAgentRestConfig(t *testing.T) {
	ns := "default"
	tests := []struct {
		name      string
		configmap *corev1.ConfigMap
		wantQPS   float32
		wantBurst int
	}{
		{
			name: "have configmap with qps and butst",
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      operatorconstants.ControllerConfig,
				},
				Data: map[string]string{
					"agentQPS":   "100",
					"agentBurst": "200",
				},
			},
			wantQPS:   100,
			wantBurst: 200,
		},
		{
			name:      "do not have configmap",
			configmap: nil,
			wantQPS:   150,
			wantBurst: 300,
		},
		{
			name: "have configmap with qps and no burst",
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      operatorconstants.ControllerConfig,
				},
				Data: map[string]string{
					"agentQPS": "100",
				},
			},

			wantQPS:   100,
			wantBurst: 300,
		},
		{
			name: "have configmap with no qps and have burst",
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      operatorconstants.ControllerConfig,
				},
				Data: map[string]string{
					"agentBurst": "100",
				},
			},
			wantQPS:   150,
			wantBurst: 100,
		},
		{
			name: "have configmap with no qps and no burst",
			configmap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: ns,
					Name:      operatorconstants.ControllerConfig,
				},
				Data: map[string]string{
					"test": "100",
				},
			},
			wantQPS:   150,
			wantBurst: 300,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &HohAgentAddon{
				ctx: context.Background(),
			}
			gotQPS, gotBurst := a.getAgentRestConfig(tt.configmap)
			if gotQPS != tt.wantQPS {
				t.Errorf("HohAgentAddon.getAgentRestConfig() got = %v, want %v", gotQPS, tt.wantQPS)
			}
			if gotBurst != tt.wantBurst {
				t.Errorf("HohAgentAddon.getAgentRestConfig() got = %v, want %v", gotQPS, tt.wantBurst)
			}
		})
	}
}
