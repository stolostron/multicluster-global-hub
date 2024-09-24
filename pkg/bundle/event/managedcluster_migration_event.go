package event

import (
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type ManagedClusterMigrationFromEvent struct {
	ManagedClusters  []string                             `json:"managedclusters"`
	BootstrapSecret  *corev1.Secret                       `json:"bootstrapsecret"`
	KlusterletConfig *klusterletv1alpha1.KlusterletConfig `json:"klusterletconfig"`
}

type ManagedClusterMigrationToEvent struct {
	ManagedServiceAccountName string `json:"managedserviceaccountname"`
	ServiceAccountNamespace   string `json:"serviceaccountnamespace"`
}
