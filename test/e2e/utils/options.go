package utils

type OptionsContainer struct {
	Options Options `yaml:"options"`
}

// Define options available for Tests to consume
type Options struct {
	GlobalHub GlobalHub `yaml:"globalhub"`
}

// Define the shape of clusters that may be added under management
type GlobalHub struct {
	Name             string       `yaml:"name,omitempty"`
	Namespace        string       `yaml:"namespace,omitempty"`
	ApiServer        string       `yaml:"apiServer,omitempty"`
	KubeConfig       string       `yaml:"kubeconfig,omitempty"`
	KubeContext      string       `yaml:"kubecontext,omitempty"`
	Nonk8sApiServer  string       `yaml:"nonk8sApiServer,omitempty"`
	ManagerImageREF  string       `yaml:"managerImageREF,omitempty"`
	AgentImageREF    string       `yaml:"agentImageREF,omitempty"`
	OperatorImageREF string       `yaml:"operatorImageREF,omitempty"`
	ManagedHubs      []ManagedHub `yaml:"managedhubs"`
}

type ManagedHub struct {
	Name            string           `yaml:"name,omitempty"`
	KubeConfig      string           `yaml:"kubeconfig,omitempty"`
	KubeContext     string           `yaml:"kubecontext,omitempty"`
	ManagedClusters []ManagedCluster `yaml:"managedclusters,omitempty"`
}

type ManagedCluster struct {
	Name        string `yaml:"name,omitempty"`
	LeafHubName string `yaml:"leafhubname,omitempty"`
	KubeConfig  string `yaml:"kubeconfig,omitempty"`
	KubeContext string `yaml:"kubecontext,omitempty"`
}
