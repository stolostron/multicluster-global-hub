package migration

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// Migration CloudEvents use a single event type (enum.ManagedClusterMigrationType) with common metadata
// carried in CloudEvents extensions:
//   - migrationid:    unique migration identifier (CR UID), for filtering unrelated migrations
//   - migrationstage: current stage (validating/initializing/deploying/registering/cleaning/rollbacking)
//   - expiretime:     RFC3339 timestamp after which receivers should discard the event
//   - clustername:    message target (the hub that should process this event)
//   - source:         message sender (built-in CloudEvents field)
//
// The payload (data) varies by direction and is differentiated by the receiver using
// evt.Source() and payload structure.

// MigrationSourceBundle defines the spec payload from manager to the source cluster.
// The "toHub" field distinguishes this from MigrationTargetBundle at the receiver side.
// MigrationId and Stage are carried in CloudEvents extensions (not serialized in JSON).
type MigrationSourceBundle struct {
	MigrationId     string         `json:"-"` // populated from extension at receiver side
	Stage           string         `json:"-"` // populated from extension at receiver side
	ToHub           string         `json:"toHub"`
	PlacementName   string         `json:"placementName"`
	ManagedClusters []string       `json:"managedClusters,omitempty"`
	BootstrapSecret *corev1.Secret `json:"bootstrapSecret,omitempty"`
	// Indicates which stage is being rolled back
	RollbackStage             string `json:"rollbackStage,omitempty"`
	RollbackingTimeoutMinutes int    `json:"rollbackingTimeoutMinutes,omitempty"`
}

// MigrationTargetBundle defines the spec payload from manager to the target cluster.
// MigrationId and Stage are carried in CloudEvents extensions (not serialized in JSON).
type MigrationTargetBundle struct {
	MigrationId                           string   `json:"-"` // populated from extension at receiver side
	Stage                                 string   `json:"-"` // populated from extension at receiver side
	ManagedServiceAccountName             string   `json:"managedServiceAccountName"`
	ManagedServiceAccountInstallNamespace string   `json:"installNamespace,omitempty"`
	ManagedClusters                       []string `json:"managedClusters,omitempty"`
	RollbackStage                         string   `json:"rollbackStage,omitempty"`
	RegisteringTimeoutMinutes             int      `json:"registeringTimeoutMinutes,omitempty"`
}

// MigrationStatusBundle is the status payload sent from managed hubs to the global hub.
// MigrationId and Stage are carried in CloudEvents extensions (not serialized in JSON).
type MigrationStatusBundle struct {
	MigrationId     string            `json:"-"` // populated from extension at receiver side
	Stage           string            `json:"-"` // populated from extension at receiver side
	ErrMessage      string            `json:"errMessage,omitempty"`
	Resync          bool              `json:"resync,omitempty"`
	ManagedClusters []string          `json:"managedClusters,omitempty"`
	ClusterErrors   map[string]string `json:"clusterErrors,omitempty"`
	// FailedClusters contains the list of clusters that failed to migrate.
	// This is sent by the target hub during registering stage rollback so the manager
	// can immediately send the rollback request to source hub for these clusters
	// without waiting for target hub rollback to complete.
	FailedClusters []string `json:"failedClusters,omitempty"`
	// FailedClustersReported indicates this bundle contains the failed clusters query result.
	// When true, the manager should process FailedClusters field to determine which clusters
	// need rollback on source hub.
	FailedClustersReported bool `json:"failedClustersReported,omitempty"`
}

// MigrationResourceBundle is the resource payload sent from source agent to target agent during deploying.
// MigrationId is carried in CloudEvents extensions (not serialized in JSON).
type MigrationResourceBundle struct {
	MigrationId               string                     `json:"-"` // populated from extension at receiver side
	TotalClusters             int                        `json:"totalClusters"`
	MigrationClusterResources []MigrationClusterResource `json:"migrationClusterResources"`
}

type MigrationClusterResource struct {
	ClusterName  string                      `json:"clusterName"`
	ResourceList []unstructured.Unstructured `json:"resourceList"`
}

// MaxMigrationBundleBytes defines the maximum size (in bytes) of the JSON-encoded MigrationResourceBundle.
// This limit prevents exceeding Kafka broker message size limits during migration operations.
const MaxMigrationBundleBytes = 800 * 1024 // 800 KiB

var log = logger.DefaultZapLogger()

// NewMigrationResourceBundle creates a new MigrationResourceBundle
func NewMigrationResourceBundle(totalClusters int) *MigrationResourceBundle {
	return &MigrationResourceBundle{
		TotalClusters:             totalClusters,
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
// Returns (true, nil) if adding would exceed size limit.
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
			log.Warnf("resource size for cluster %s exceeds the bundle limit: %d bytes. Please increase the Kafka message"+
				" size limit to avoid potential data loss.", resource.ClusterName, size)
			return true, nil
		}
		b.MigrationClusterResources = b.MigrationClusterResources[:len(b.MigrationClusterResources)-1]
		return false, nil
	}

	return true, nil
}
