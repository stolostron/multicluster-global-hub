package config

import (
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const EnvKubconfig = "KUBECONFIG"

func loadDynamicKubeConfig(envVar string) (*rest.Config, error) {
	kubeconfigPath := os.Getenv(envVar)
	if kubeconfigPath != "" {
		// Load kubeconfig from the specified path
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	// Use the in-cluster configuration
	return config.GetConfig()
}

func DefaultKubeConfig() (*rest.Config, error) {
	return loadDynamicKubeConfig(EnvKubconfig)
}
