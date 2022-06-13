package utils

const HUB_OF_HUB_CLUSTER_NAME = "hub-of-hub-cluster"

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
	Name           string `yaml:"name,omitempty"`
	Namespace      string `yaml:"namespace,omitempty"`
	BaseDomain     string `yaml:"baseDomain,omitempty"`
	KubeConfig     string `yaml:"kubeconfig,omitempty"`
	KubeContext    string `yaml:"kubecontext,omitempty"`
	MasterURL      string `yaml:"masterURL,omitempty"`
	DatabaseSecret string `yaml:"databaseSecret,omitempty"`
}

type ManagedCluster struct {
	Name        string `yaml:"name,omitempty"`
	LeafHubName string `yaml:"leafhubname,omitempty"`
	KubeConfig  string `yaml:"kubeconfig,omitempty"`
	KubeContext string `yaml:"kubecontext,omitempty"`
}
