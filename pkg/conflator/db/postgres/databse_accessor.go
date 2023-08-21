package postgres

import (
	"context"
)

// StatusTransportBridgeDB is the db interface required by status transport bridge.
type StatusTransportBridgeDB interface {
	GetPoolSize() int32
	Stop()

	ManagedClustersStatusDB
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
