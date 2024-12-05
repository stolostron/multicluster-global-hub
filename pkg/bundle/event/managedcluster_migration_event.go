package event

import (
	corev1 "k8s.io/api/core/v1"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
)

type ManagedClusterMigrationFromEvent struct {
	ManagedClusters  []string                             `json:"managedClusters"`
	BootstrapSecret  *corev1.Secret                       `json:"bootstrapSecret"`
	KlusterletConfig *klusterletv1alpha1.KlusterletConfig `json:"klusterletConfig"`
}

type ManagedClusterMigrationToEvent struct {
	ManagedServiceAccountName             string                         `json:"managedServiceAccountName"`
	ManagedServiceAccountInstallNamespace string                         `json:"managedServiceAccountInstallNamespace"`
	KlusterletAddonConfig                 *addonv1.KlusterletAddonConfig `json:"klusterletAddonConfig"`
}
