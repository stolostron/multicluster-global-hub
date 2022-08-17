// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
)

// DB is the needed interface for the spec/status syncer and spec/status transport
type DB interface {
	// GetConn returns the database connction
	// TODO(morvencao): should not return concrete DB connection type here,
	// maybe should return a warpe type for DB connection
	GetConn() *pgxpool.Pool

	SpecDB
	StatusDB
}

// SpecDB is the needed interface for the spec syncer and spec transport
type SpecDB interface {
	// GetLastUpdateTimestamp returns the last update timestamp of a specific table.
	GetLastUpdateTimestamp(ctx context.Context, tableName string, filterLocalResources bool) (*time.Time, error)

	ObjectsSpecDB
	ManagedClusterLabelsSpecDB
}

// ObjectsSpecDB is the interface needed by the spec syncer and spec transport bridge to and from sync objects tables.
type ObjectsSpecDB interface {
	// CURD operations
	QuerySpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error
	InsertSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error
	UpdateSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error
	DeleteSpecObject(ctx context.Context, tableName, name, namespace string) error

	// GetObjectsBundle returns a bundle of objects from a specific table.
	GetObjectsBundle(ctx context.Context, tableName string, createObjFunc bundle.CreateObjectFunction,
		intoBundle bundle.ObjectsBundle) (*time.Time, error)
}

// ManagedClusterLabelsSpecDB is the interface needed by the spec transport bridge to sync managed-cluster labels table.
type ManagedClusterLabelsSpecDB interface {
	// GetUpdatedManagedClusterLabelsBundles returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects
	// belonging to a leaf-hub that had at least one update since the given timestamp, from a specific table.
	GetUpdatedManagedClusterLabelsBundles(ctx context.Context, tableName string,
		timestamp *time.Time) (map[string]*spec.ManagedClusterLabelsSpecBundle, error)
	// GetEntriesWithDeletedLabels returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects that have a
	// none-empty deleted-label-keys column.
	GetEntriesWithDeletedLabels(ctx context.Context,
		tableName string) (map[string]*spec.ManagedClusterLabelsSpecBundle, error)
	// UpdateDeletedLabelKeys updates
	UpdateDeletedLabelKeys(ctx context.Context, tableName string, readVersion int64, leafHubName string,
		managedClusterName string, deletedLabelKeys []string) error
	TempManagedClusterLabelsSpecDB
}

// TempManagedClusterLabelsSpecDB appends ManagedClusterLabelsSpecDB interface with temporary functionality that should
// be removed after it is satisfied by a different component.
// TODO: once non-k8s-restapi exposes hub names, delete interface.
type TempManagedClusterLabelsSpecDB interface {
	// GetEntriesWithoutLeafHubName returns a slice of ManagedClusterLabelsSpec that are missing leaf hub name.
	GetEntriesWithoutLeafHubName(ctx context.Context, tableName string) (
		[]*spec.ManagedClusterLabelsSpec, error)
	// UpdateLeafHubName updates leaf hub name for a given managed cluster under optimistic concurrency.
	UpdateLeafHubName(ctx context.Context, tableName string, readVersion int64,
		managedClusterName string, leafHubName string) error
}

// StatusDB is the needed interface for the db transport bridge to fetch information from status DB.
type StatusDB interface {
	// GetManagedClusterLabelsStatus gets the labels present in managed-cluster CR metadata from a specific table.
	GetManagedClusterLabelsStatus(ctx context.Context, tableName string, leafHubName string,
		managedClusterName string) (map[string]string, error)
	TempStatusDB
}

// TempStatusDB appends StatusDB interface with temporary functionality that should be removed after it is satisfied
// by a different component.
// TODO: once non-k8s-restapi exposes hub names, delete interface.
type TempStatusDB interface {
	// GetManagedClusterLeafHubName returns leaf-hub name for a given managed cluster from a specific table.
	GetManagedClusterLeafHubName(ctx context.Context, tableName string, managedClusterName string) (string, error)
}
