package migration

import (
	"encoding/json"
	"fmt"

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
	ClusterName  string                      `json:"clusterName"`
	ResourceList []unstructured.Unstructured `json:"resourcesList"`
}

// MaxMigrationBundleBytes defines the maximum size (in bytes) of the JSON-encoded MigrationResourceBundle.
// This limit prevents exceeding Kafka broker message size limits during migration operations.
const MaxMigrationBundleBytes = 800 * 1024 // 800 KiB

// CloudEvents extension keys for migration batch tracking
const (
	ExtTotalClusters = "totalclusters" // Total number of clusters to be migrated
)

// NewMigrationResourceBundle creates a new MigrationResourceBundle
func NewMigrationResourceBundle(migrationId string) *MigrationResourceBundle {
	return &MigrationResourceBundle{
		MigrationId:               migrationId,
		MigrationClusterResources: []MigrationClusterResource{},
	}
}

// Size returns the size in bytes of the JSON-encoded MigrationResourceBundle
func (b *MigrationResourceBundle) Size() (int, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

// IsEmpty returns true if the bundle contains no cluster resources
func (b *MigrationResourceBundle) IsEmpty() bool {
	return len(b.MigrationClusterResources) == 0
}

// Clean clears all cluster resources from the bundle
func (b *MigrationResourceBundle) Clean() {
	b.MigrationClusterResources = nil
}

// AddClusterResource tries to add a cluster resource to the bundle.
// Returns (true, nil) if added successfully.
// Returns (false, nil) if adding would exceed size limit.
// Returns (false, error) if there's an error or single resource is too large.
func (b *MigrationResourceBundle) AddClusterResource(resource MigrationClusterResource) (bool, error) {
	wasEmptyBeforeAdd := b.IsEmpty()

	b.MigrationClusterResources = append(b.MigrationClusterResources, resource)

	size, err := b.Size()
	if err != nil {
		b.MigrationClusterResources = b.MigrationClusterResources[:len(b.MigrationClusterResources)-1]
		return false, err
	}

	if size > MaxMigrationBundleBytes {
		if wasEmptyBeforeAdd {
			return false, fmt.Errorf("single cluster resource exceeds bundle size limit for cluster %s: %d bytes",
				resource.ClusterName, size)
		}
		b.MigrationClusterResources = b.MigrationClusterResources[:len(b.MigrationClusterResources)-1]
		return false, nil
	}

	return true, nil
}
