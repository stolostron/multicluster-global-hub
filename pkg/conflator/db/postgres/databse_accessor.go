package postgres

import (
	"context"

	set "github.com/deckarep/golang-set"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// StatusTransportBridgeDB is the db interface required by status transport bridge.
type StatusTransportBridgeDB interface {
	GetPoolSize() int32
	Stop()

	ManagedClustersStatusDB
	PoliciesStatusDB
	AggregatedPoliciesStatusDB
	GenericStatusResourceDB
	LocalPoliciesStatusDB
	ControlInfoDB
}

// BatchSenderDB is the db interface required for sending batch updates.
type BatchSenderDB interface {
	// SendBatch sends a batch operation to the db and returns list of errors if there were any.
	SendBatch(ctx context.Context, batch interface{}) error
}

// ManagedClustersStatusDB is the db interface required to manage managed clusters status.
type ManagedClustersStatusDB interface {
	BatchSenderDB
	// GetManagedClustersByLeafHub returns a map from clusterName to its resourceVersion.
	GetManagedClustersByLeafHub(ctx context.Context, schema string, tableName string,
		leafHubName string) (map[string]string, error)
	// NewManagedClustersBatchBuilder returns managed clusters batch builder.
	NewManagedClustersBatchBuilder(schema string, tableName string, leafHubName string) ManagedClustersBatchBuilder
}

// PoliciesStatusDB is the db interface required to manage policies status.
type PoliciesStatusDB interface {
	BatchSenderDB
	// GetComplianceStatusByLeafHub returns a map of policies, each maps to a set of clusters.
	GetComplianceStatusByLeafHub(ctx context.Context, schema string, tableName string,
		leafHubName string) (map[string]*PolicyClustersSets, error)
	// GetNonCompliantClustersByLeafHub returns a map of policies, each maps to sets of (NonCompliant,Unknown) clusters.
	GetNonCompliantClustersByLeafHub(ctx context.Context, schema string, tableName string,
		leafHubName string) (map[string]*PolicyClustersSets, error)
	// NewPoliciesBatchBuilder returns policies status batch builder.
	NewPoliciesBatchBuilder(schema string, tableName string, leafHubName string) PoliciesBatchBuilder
}

// AggregatedPoliciesStatusDB is the db interface required to manage aggregated policy info.
type AggregatedPoliciesStatusDB interface {
	GetPolicyIDsByLeafHub(ctx context.Context, schema string, tableName string, leafHubName string) (set.Set, error)
	InsertOrUpdateAggregatedPolicyCompliance(ctx context.Context, schema string, tableName string, leafHubName string,
		policyID string, appliedClusters int, nonCompliantClusters int) error
	DeleteAllComplianceRows(ctx context.Context, schema string, tableName string, leafHubName string,
		policyID string) error
}

// GenericStatusResourceDB is the db interface required to manage generic status resources.
type GenericStatusResourceDB interface {
	BatchSenderDB
	// GetResourceIDToVersionByLeafHub returns a map from resource id to its resourceVersion.
	GetResourceIDToVersionByLeafHub(ctx context.Context, schema string, tableName string,
		leafHubName string) (map[string]string, error)
	NewGenericBatchBuilder(schema string, tableName string, leafHubName string) GenericBatchBuilder
}

// LocalPoliciesStatusDB is the db interface required to manage local policies.
type LocalPoliciesStatusDB interface {
	BatchSenderDB
	// GetLocalResourceIDToVersionByLeafHub returns a map from local resource id to its resourceVersion.
	GetLocalResourceIDToVersionByLeafHub(ctx context.Context, schema string, tableName string,
		leafHubName string) (map[string]string, error)
	// NewGenericLocalBatchBuilder returns generic local batch builder.
	NewGenericLocalBatchBuilder(schema string, tableName string, leafHubName string) GenericLocalBatchBuilder
}

// ControlInfoDB is the db interface required to manage control info status.
type ControlInfoDB interface {
	// UpdateHeartbeat inserts or updates heartbeat for a leaf hub.
	UpdateHeartbeat(ctx context.Context, schema string, tableName string, leafHubName string) error
}

// NewPolicyClusterSets creates a new instance of PolicyClustersSets.
func NewPolicyClusterSets() *PolicyClustersSets {
	return &PolicyClustersSets{
		complianceToSetMap: map[database.ComplianceStatus]set.Set{
			database.Compliant:    set.NewSet(),
			database.NonCompliant: set.NewSet(),
			database.Unknown:      set.NewSet(),
		},
	}
}

// PolicyClustersSets is a data structure to hold both non compliant clusters set and unknown clusters set.
type PolicyClustersSets struct {
	complianceToSetMap map[database.ComplianceStatus]set.Set
}

// AddCluster adds the given cluster name to the given compliance status clusters set.
func (sets *PolicyClustersSets) AddCluster(clusterName string, complianceStatus database.ComplianceStatus) {
	sets.complianceToSetMap[complianceStatus].Add(clusterName)
}

// GetAllClusters returns the clusters set of a policy (union of compliant/nonCompliant/unknown clusters).
func (sets *PolicyClustersSets) GetAllClusters() set.Set {
	return sets.complianceToSetMap[database.Compliant].
		Union(sets.complianceToSetMap[database.NonCompliant].
			Union(sets.complianceToSetMap[database.Unknown]))
}

// GetClusters returns the clusters set by compliance status.
func (sets *PolicyClustersSets) GetClusters(complianceStatus database.ComplianceStatus) set.Set {
	return sets.complianceToSetMap[complianceStatus]
}
