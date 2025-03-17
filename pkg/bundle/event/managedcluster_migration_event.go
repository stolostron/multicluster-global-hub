package event

import (
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
)

type ManagedClusterMigrationFromEvent struct {
	// Allowed values: "initializing", "registering"
	Stage           string         `json:"stage"`
	ToHub           string         `json:"toHub"`
	ManagedClusters []string       `json:"managedClusters,omitempty"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret,omitempty"`
	// Deprecated: generate in the 
	KlusterletConfig *klusterletv1alpha1.KlusterletConfig `json:"klusterletConfig,omitempty"`
}

type ManagedClusterMigrationToEvent struct {
	ManagedServiceAccountName             string                         `json:"managedServiceAccountName"`
	ManagedServiceAccountInstallNamespace string                         `json:"managedServiceAccountInstallNamespace"`
	KlusterletAddonConfig                 *addonv1.KlusterletAddonConfig `json:"klusterletAddonConfig"`
}
