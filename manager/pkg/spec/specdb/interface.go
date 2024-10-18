// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package specdb

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers/bundle"
)

// SpecDB is the needed interface for the spec syncer and spec transport
type SpecDB interface {
	// GetLastUpdateTimestamp returns the last update timestamp of a specific table.
	GetLastUpdateTimestamp(ctx context.Context, tableName string, filterLocalResources bool) (*time.Time, error)
	ObjectsSpecDB
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
