package main

import (
	"context"
	"os"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	config "github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestParseFlags(t *testing.T) {
	// Save original command-line arguments
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	// Set up test arguments
	os.Args = []string{
		"cmd",
		"--leaf-hub-name=test-hub",
		"--pod-namespace=test-namespace",
		"--consumer-worker-pool-size=5",
	}

	agentConfig := parseFlags()

	assert.Equal(t, "test-hub", agentConfig.LeafHubName)
	assert.Equal(t, "test-namespace", agentConfig.PodNamespace)
	assert.Equal(t, 5, agentConfig.SpecWorkPoolSize)
}

func TestCompleteConfig(t *testing.T) {
	testCases := []struct {
		name           string
		agentConfig    *config.AgentConfig
		fakeClient     client.Client
		expectConfig   *config.AgentConfig
		expectErrorMsg string
	}{
		{
			name: "Invalid leaf-hub-name without standalone mode",
			agentConfig: &config.AgentConfig{
				LeafHubName: "",
				DeployMode:  string(constants.DefaultMode),
			},
			fakeClient: fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects(&configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec:       configv1.ClusterVersionSpec{ClusterID: configv1.ClusterID("")},
			}).Build(),
			expectErrorMsg: "the leaf-hub-name must not be empty",
		},
		{
			name: "Empty leaf-hub-name(clusterId) with standalone mode",
			agentConfig: &config.AgentConfig{
				LeafHubName: "",
				DeployMode:  string(constants.StandaloneMode),
			},
			fakeClient: fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects(&configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec:       configv1.ClusterVersionSpec{ClusterID: configv1.ClusterID("")},
			}).Build(),
			expectErrorMsg: "the clusterId from ClusterVersion must not be empty",
		},
		{
			name: "Invalid leaf-hub-name(clusterId) under standalone mode",
			agentConfig: &config.AgentConfig{
				LeafHubName: "",
				DeployMode:  string(constants.StandaloneMode),
			},
			fakeClient:     fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).Build(),
			expectErrorMsg: "clusterversions.config.openshift.io \"version\" not found",
		},
		{
			name: "Valid configuration under standalone mode",
			agentConfig: &config.AgentConfig{
				LeafHubName:      "",
				DeployMode:       string(constants.StandaloneMode),
				SpecWorkPoolSize: 5,
				TransportConfig: &transport.TransportInternalConfig{
					ConsumerGroupId: "test-hub",
					TransportType:   string(transport.Kafka),
				},
			},
			fakeClient: fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects(&configv1.ClusterVersion{
				ObjectMeta: metav1.ObjectMeta{Name: "version"},
				Spec:       configv1.ClusterVersionSpec{ClusterID: configv1.ClusterID("123")},
			}).Build(),
			expectConfig: &config.AgentConfig{
				LeafHubName:      "123",
				DeployMode:       string(constants.StandaloneMode),
				SpecWorkPoolSize: 5,
				MetricsAddress:   "0.0.0.0:8384",
				TransportConfig: &transport.TransportInternalConfig{
					ConsumerGroupId: "123",
					TransportType:   string(transport.Kafka),
				},
			},
		},
		{
			name: "Invalid work pool size",
			agentConfig: &config.AgentConfig{
				LeafHubName: "hub1",
				DeployMode:  string(constants.DefaultMode),
				TransportConfig: &transport.TransportInternalConfig{
					TransportType: string(transport.Kafka),
				},
			},
			fakeClient:     fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects().Build(),
			expectErrorMsg: "flag consumer-worker-pool-size should be in the scope [1, 100]",
		},
		{
			name: "Valid configuration without standalone mode",
			agentConfig: &config.AgentConfig{
				LeafHubName:      "hub1",
				DeployMode:       string(constants.DefaultMode),
				SpecWorkPoolSize: 5,
				TransportConfig: &transport.TransportInternalConfig{
					TransportType: string(transport.Kafka),
				},
			},
			fakeClient: fake.NewClientBuilder().WithScheme(config.GetRuntimeScheme()).WithObjects().Build(),
			expectConfig: &config.AgentConfig{
				LeafHubName:      "hub1",
				DeployMode:       string(constants.DefaultMode),
				SpecWorkPoolSize: 5,
				MetricsAddress:   "0.0.0.0:8384",
				TransportConfig: &transport.TransportInternalConfig{
					ConsumerGroupId: "hub1",
					TransportType:   string(transport.Kafka),
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := completeConfig(context.Background(), tc.fakeClient, tc.agentConfig)
			if tc.expectErrorMsg != "" {
				assert.Contains(t, err.Error(), tc.expectErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectConfig, tc.agentConfig)
				// assert.Equal(t, reflect.DeepEqual(tc.agentConfig, tc.expectConfig), true)
			}
		})
	}
}
