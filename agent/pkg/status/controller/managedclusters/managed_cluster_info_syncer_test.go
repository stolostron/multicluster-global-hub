package managedclusters

// func TestLaunchManagedClusterInfoSyncer(t *testing.T) {
// 	ctx := context.Background()
// 	agentConfig := &config.AgentConfig{
// 		TransportConfig: &transport.TransportInternalConfig{
// 			TransportType: string(transport.Rest),
// 		},
// 	}
// 	cfg := &rest.Config{
// 		Host:            "https://mock-cluster",
// 		APIPath:         "/api",
// 		BearerToken:     "mock-token",
// 		TLSClientConfig: rest.TLSClientConfig{Insecure: true},
// 	}
// 	mgr, err := manager.New(cfg, manager.Options{Scheme: config.GetRuntimeScheme()})
// 	assert.Nil(t, err)
// 	err = LaunchManagedClusterInfoSyncer(ctx, mgr, agentConfig, nil)
// 	assert.Nil(t, err)
// }
