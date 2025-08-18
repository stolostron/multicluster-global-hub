package main

import (
	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/samples/config"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getRuntimeClient() (runtimeclient.Client, error) {
	kubeconfig, err := config.DefaultKubeConfig()
	if err != nil {
		return nil, err
	}
	c, err := runtimeclient.New(kubeconfig, runtimeclient.Options{Scheme: configs.GetRuntimeScheme()})
	if err != nil {
		return nil, err
	}
	return c, nil
}
