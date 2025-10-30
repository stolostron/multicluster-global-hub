package migration

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	PlacementName   string         `json:"placementName"`
	ManagedClusters []string       `json:"managedClusters,omitempty"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret,omitempty"`
	// Indicates which stage is being rolled back
	RollbackStage             string `json:"rollbackStage,omitempty"`
	RollbackingTimeoutMinutes int    `json:"rollbackingTimeoutMinutes,omitempty"`
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
	MigrationId     string            `json:"migrationId"`
	Stage           string            `json:"stage"`
	ErrMessage      string            `json:"errMessage,omitempty"`
	Resync          bool              `json:"resync,omitempty"`
	ManagedClusters []string          `json:"managedClusters,omitempty"`
	ClusterErrors   map[string]string `json:"clusterErrors,omitempty"`
}

type MigrationResourceBundle struct {
	MigrationId               string                     `json:"migrationId"`
	MigrationClusterResources []MigrationClusterResource `json:"migrationClusterResources"`
}

type MigrationClusterResource struct {
	ClusterName string                      `json:"clusterName"`
	ResouceList []unstructured.Unstructured `json:"resourcesList"`
}
