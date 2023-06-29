package utils

type LocalOptionsContainer struct {
	LocalOptions LocalOptions `yaml:"options"`
}

// Define options available for Tests to consume
type LocalOptions struct {
	LocalHubCluster      localHOHCluster       `yaml:"hub"`
	LocalManagedClusters []LocalManagedCluster `yaml:"clusters"`
}

// Define the shape of clusters that may be added under management
type localHOHCluster struct {
	Name            string `yaml:"name,omitempty"`
	Namespace       string `yaml:"namespace,omitempty"`
	ApiServer       string `yaml:"apiServer,omitempty"`
	KubeConfig      string `yaml:"kubeconfig,omitempty"`
	KubeContext     string `yaml:"kubecontext,omitempty"`
	Nonk8sApiServer string `yaml:"nonk8sApiServer,omitempty"`
	DatabaseURI     string `yaml:"databaseURI,omitempty"`
	StoragePath     string `yaml:"storagePath,omitempty"`
	TransportPath   string `yaml:"transportPath,omitempty"`
	CrdsDir         string `yaml:"crdsDir,omitempty"`
	DatabaseExternalHost string `yaml:"databaseExternalHost,omitempty"`
	DatabaseExternalPort int    `yaml:"databaseExternalPort,omitempty"`
	ManagerImageREF   string `yaml:"ManagerImageREF,omitempty"`
	AgentImageREF     string `yaml:"AgentImageREF,omitempty"`
	OperatorImageREF     string `yaml:"OperatorImageREF,omitempty"`
}

type LocalManagedCluster struct {
	Name        string `yaml:"name,omitempty"`
	LeafHubName string `yaml:"leafhubname,omitempty"`
	KubeConfig  string `yaml:"kubeconfig,omitempty"`
	KubeContext string `yaml:"kubecontext,omitempty"`
}
