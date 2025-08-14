package migration

import (
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	corev1 "k8s.io/api/core/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

// Since Kafka persists messages in a topic, multiple migration processes running in the system might all use the
// same topic to send messages. This can lead to an issue where a downstream service (or "hub") receives events
// from a previous migration process.
// To address this, we’ve introduced the following mechanisms:
// 1. Message expiration: Ensure that migration messages on the topic expire after 10 minutes, which aligns with the
// typical timeout of a migration process.
// 2. Migration ID tagging: Include a unique migration ID with each event. This allows receivers to process only the
// events relevant to their migration and ignore others.

// MigrationSourceBundle defines the resources from migration controller to the source cluster
type MigrationSourceBundle struct {
	MigrationId     string         `json:"migrationId"`
	Stage           string         `json:"stage"`
	ToHub           string         `json:"toHub"`
	ManagedClusters []string       `json:"managedClusters,omitempty"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret,omitempty"`
	RollbackStage   string         `json:"rollbackStage,omitempty"` // Indicates which stage is being rolled back
}

// MigrationTargetBundle defines the resources from migration controllers to the target cluster
type MigrationTargetBundle struct {
	MigrationId                           string   `json:"migrationId"`
	Stage                                 string   `json:"stage"`
	ManagedServiceAccountName             string   `json:"managedServiceAccountName"`
	ManagedServiceAccountInstallNamespace string   `json:"installNamespace,omitempty"`
	ManagedClusters                       []string `json:"managedClusters,omitempty"`
	RollbackStage                         string   `json:"rollbackStage,omitempty"`
	RegisteringTimeoutMinutes             int      `json:"registeringTimeoutMinutes,omitempty"`
}

// The bundle sent from the managed hubs to the global hub
type MigrationStatusBundle struct {
	MigrationId string `json:"migrationId"`
	Stage       string `json:"stage"`
	ErrMessage  string `json:"errMessage,omitempty"`
	// ManagedClusters []string `json:"managedClusters,omitempty"`
}

type MigrationResourceBundle struct {
	MigrationId           string                          `json:"migrationId"`
	ManagedClusters       []clusterv1.ManagedCluster      `json:"managedClusters,omitempty"`
	KlusterletAddonConfig []addonv1.KlusterletAddonConfig `json:"klusterletAddonConfigs,omitempty"`
}
