package managedclusters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestLaunchManagedClusterInfoSyncer(t *testing.T) {
	ctx := context.Background()
	agentConfig := &config.AgentConfig{
		TransportConfig: &transport.TransportInternalConfig{
			TransportType: string(transport.Rest),
		},
	}
	cfg := &rest.Config{
		Host:            "https://mock-cluster",
		APIPath:         "/api",
		BearerToken:     "mock-token",
		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
	}
	mgr, err := manager.New(cfg, manager.Options{Scheme: config.GetRuntimeScheme()})
	assert.Nil(t, err)
	err = LaunchManagedClusterInfoSyncer(ctx, mgr, agentConfig, nil)
	assert.Nil(t, err)
}
