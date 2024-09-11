package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
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
		"--transport-type=kafka",
		"--consumer-worker-pool-size=5",
	}

	agentConfig := parseFlags()

	assert.Equal(t, "test-hub", agentConfig.LeafHubName)
	assert.Equal(t, "test-namespace", agentConfig.PodNamespace)
	assert.Equal(t, "kafka", agentConfig.TransportConfig.TransportType)
	assert.Equal(t, 5, agentConfig.SpecWorkPoolSize)
}

func TestCompleteConfig(t *testing.T) {
	testCases := []struct {
		name           string
		agentConfig    *config.AgentConfig
		expectConfig   *config.AgentConfig
		expectErrorMsg string
	}{
		{
			name: "Invalid leaf-hub-name",
			agentConfig: &config.AgentConfig{
				LeafHubName: "",
			},
			expectErrorMsg: "flag leaf-hub-name can't be empty",
		},
		{
			name: "Standalone configuration",
			agentConfig: &config.AgentConfig{
				LeafHubName:      "test-hub",
				Standalone:       true,
				SpecWorkPoolSize: 10,
				TransportConfig: &transport.TransportConfig{
					ConsumerGroupId: "test-hub",
				},
			},
			expectConfig: &config.AgentConfig{
				LeafHubName:      "test-hub",
				Standalone:       true,
				SpecWorkPoolSize: 0,
				MetricsAddress:   fmt.Sprintf("%s:%d", metricsHost, metricsPort),
				TransportConfig: &transport.TransportConfig{
					ConsumerGroupId: "",
					TransportType:   string(transport.Rest),
				},
			},
			expectErrorMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := completeConfig(tc.agentConfig)
			if tc.expectErrorMsg != "" {
				assert.Equal(t, err.Error(), tc.expectErrorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, reflect.DeepEqual(tc.agentConfig, tc.expectConfig), true)
			}
		})
	}
}
