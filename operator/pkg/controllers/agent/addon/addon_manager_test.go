package addon

import (
	"encoding/base64"
	"testing"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/certificates"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	operatorconstants "github.com/stolostron/multicluster-global-hub/operator/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGetRestConfig(t *testing.T) {
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
			config.SetControllerConfig(tt.configmap)
			gotQPS, gotBurst := config.GetAgentRestConfig()
			if gotQPS != tt.wantQPS {
				t.Errorf("HohAgentAddon.getAgentRestConfig() got = %v, want %v", gotQPS, tt.wantQPS)
			}
			if gotBurst != tt.wantBurst {
				t.Errorf("HohAgentAddon.getAgentRestConfig() got = %v, want %v", gotQPS, tt.wantBurst)
			}
		})
	}
}

func TestGetInventoryCredential(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = routev1.AddToScheme(scheme)

	namespace := "test-namespace"
	config.SetMGHNamespacedName(types.NamespacedName{Namespace: namespace, Name: "testmgh"})

	caSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      certificates.InventoryServerCASecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"tls.crt": []byte("test-ca-cert"),
			"tls.key": []byte("test-ca-key"),
		},
	}

	inventoryRoute := &routev1.Route{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.InventoryRouteName,
			Namespace: namespace,
		},
		Spec: routev1.RouteSpec{
			Host: "inventory.example.com",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(caSecret, inventoryRoute).Build()

	// Test
	result, err := getInventoryCredential(client)
	assert.Nil(t, err)
	assert.NotNil(t, result)

	expectedCredential := &transport.RestfulConfig{
		Host:             "https://inventory.example.com:443",
		CASecretName:     certificates.InventoryServerCASecretName,
		CACert:           base64.StdEncoding.EncodeToString([]byte("test-ca-cert")),
		ClientSecretName: config.AgentCertificateSecretName(),
	}

	assert.Equal(t, expectedCredential, result)
}

func TestGetInventoryCredentialError(t *testing.T) {
	// Setup
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = routev1.AddToScheme(scheme)

	namespace := "test-namespace"
	config.SetMGHNamespacedName(types.NamespacedName{Namespace: namespace, Name: "testmgh"})

	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Test
	result, err := getInventoryCredential(client)

	// Assert
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to get the inventory server ca")
}
