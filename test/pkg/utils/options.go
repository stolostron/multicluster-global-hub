package utils

type OptionsContainer struct {
	Options Options `yaml:"options"`
}

// Define options available for Tests to consume
type Options struct {
	HubCluster      HOHCluster       `yaml:"hub"`
	ManagedClusters []ManagedCluster `yaml:"clusters"`
}

// Define the shape of clusters that may be added under management
type HOHCluster struct {
	KubeConfig      string `yaml:"kubeconfig,omitempty"`
}

type ManagedCluster struct {
	KubeConfig  string `yaml:"kubeconfig,omitempty"`
}
