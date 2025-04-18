package migration

import (
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// ManagedClusterMigrationFromEvent defines the resources from migration controller to the source cluster
type ManagedClusterMigrationFromEvent struct {
	Stage           string         `json:"stage"`
	ToHub           string         `json:"toHub"`
	ManagedClusters []string       `json:"managedClusters,omitempty"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret,omitempty"`
}

// ManagedClusterMigrationToEvent defines the resources from migration controllers to the target cluster
type ManagedClusterMigrationToEvent struct {
	Stage                                 string                         `json:"stage"`
	ManagedServiceAccountName             string                         `json:"managedServiceAccountName"`
	ManagedServiceAccountInstallNamespace string                         `json:"installNamespace,omitempty"`
	KlusterletAddonConfig                 *addonv1.KlusterletAddonConfig `json:"klusterletAddonConfig,omitempty"`
}

// Confiramtion from the ManagedHub: Initialized, Cleanup, Deployed
type ManagedClusterMigrationBundle struct {
	Stage                 string                         `json:"stage"`
	KlusterletAddonConfig *addonv1.KlusterletAddonConfig `json:"klusterletAddonConfig,omitempty"`
	ManagedClusters       []string                       `json:"managedClusters,omitempty"`
}

type SourceClusterMigrationResources struct {
	ManagedClusters       []clusterv1.ManagedCluster      `json:"managedClusters,omitempty"`
	KlusterletAddonConfig []addonv1.KlusterletAddonConfig `json:"klusterletAddonConfigs"`
	// TODO: other resources
}
