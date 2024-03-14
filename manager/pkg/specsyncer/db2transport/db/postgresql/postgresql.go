// Copyright (c) 2020 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package postgresql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"sigs.k8s.io/controller-runtime/pkg/client"

	managerconfig "github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var errOptimisticConcurrencyUpdateFailed = errors.New("zero rows were affected by an optimistic concurrency update")

// PostgreSQL abstracts PostgreSQL client.
type PostgreSQL struct {
	conn *pgxpool.Pool
}

// NewSpecPostgreSQL creates a new instance of PostgreSQL object.
func NewSpecPostgreSQL(ctx context.Context, dataConfig *managerconfig.DatabaseConfig) (*PostgreSQL, error) {
	pool, err := database.PostgresConnPool(ctx, dataConfig.ProcessDatabaseURL, dataConfig.CACertPath,
		int32(dataConfig.MaxOpenConns))
	if err != nil {
		return nil, err
	}
	return &PostgreSQL{conn: pool}, nil
}

// Stop stops PostgreSQL and closes the connection pool.
func (p *PostgreSQL) Stop() {
	p.conn.Close()
}

// GetConn returns the current postgresql connection
func (p *PostgreSQL) GetConn() *pgxpool.Pool {
	return p.conn
}

// GetLastUpdateTimestamp returns the last update timestamp of a specific table.
func (p *PostgreSQL) GetLastUpdateTimestamp(ctx context.Context, tableName string,
	filterLocalResources bool,
) (*time.Time, error) {
	var lastTimestamp time.Time

	query := fmt.Sprintf(`SELECT MAX(updated_at) FROM spec.%s WHERE
		payload->'metadata'->'labels'->'global-hub.open-cluster-management.io/global-resource' IS NOT NULL`,
		tableName)

	if !filterLocalResources {
		query = fmt.Sprintf(`SELECT MAX(updated_at) FROM spec.%s`, tableName)
	}

	err := p.conn.QueryRow(ctx, query).Scan(&lastTimestamp)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no objects in the table spec.%s - %w", tableName, err)
	}

	return &lastTimestamp, nil
}

// QuerySpecObject gets object from given table with object UID
func (p *PostgreSQL) QuerySpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	query := fmt.Sprintf("SELECT payload FROM spec.%s WHERE id = $1", tableName)
	if err := p.conn.QueryRow(ctx, query, objUID).Scan(&object); err != nil {
		return fmt.Errorf("failed to get the instance in the database: %w", err)
	}
	return nil
}

// InsertSpecObject insets new object to given table with object UID and payload
func (p *PostgreSQL) InsertSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	query := fmt.Sprintf("INSERT INTO spec.%s (id,payload) values($1, $2::jsonb)", tableName)
	if _, err := p.conn.Exec(ctx, query, objUID, object); err != nil {
		return fmt.Errorf("insert into database failed: %w", err)
	}
	return nil
}

// UpdateSpecObject updates object payload in given table with object UID
func (p *PostgreSQL) UpdateSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	query := fmt.Sprintf("UPDATE spec.%s SET payload = $1 WHERE id = $2", tableName)
	if _, err := p.conn.Exec(ctx, query, object, objUID); err != nil {
		return fmt.Errorf("failed to update the database with new value: %w", err)
	}
	return nil
}

// DeleteSpecObject deletes object with name and namespace from given table
func (p *PostgreSQL) DeleteSpecObject(ctx context.Context, tableName, name, namespace string) error {
	var err error
	if namespace != "" {
		query := fmt.Sprintf(`UPDATE spec.%s SET deleted = true WHERE payload -> 'metadata' ->> 'name' = $1 AND
			payload -> 'metadata' ->> 'namespace' = $2 AND deleted = false`, tableName)
		_, err = p.conn.Exec(ctx, query, name, namespace)
	} else {
		query := fmt.Sprintf(`UPDATE spec.%s SET deleted = true WHERE payload -> 'metadata' ->> 'name' = $1 AND
			payload -> 'metadata' ->> 'namespace' IS NULL AND deleted = false`, tableName)
		_, err = p.conn.Exec(ctx, query, name)
	}

	if err != nil {
		return fmt.Errorf("failed to delete instance from the database: %w", err)
	}

	return nil
}

// GetObjectsBundle returns a bundle of objects from a specific table.
func (p *PostgreSQL) GetObjectsBundle(ctx context.Context, tableName string, createObjFunc bundle.CreateObjectFunction,
	intoBundle bundle.ObjectsBundle,
) (*time.Time, error) {
	timestamp, err := p.GetLastUpdateTimestamp(ctx, tableName, true)
	if err != nil {
		return nil, err
	}

	rows, err := p.conn.Query(ctx, fmt.Sprintf(`SELECT id,payload,deleted FROM spec.%s WHERE
		payload->'metadata'->'labels'->'global-hub.open-cluster-management.io/global-resource' IS NOT NULL`,
		tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to query table spec.%s - %w", tableName, err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			objID   string
			deleted bool
		)

		object := createObjFunc()
		if err := rows.Scan(&objID, &object, &deleted); err != nil {
			return nil, fmt.Errorf("error reading from table spec.%s - %w", tableName, err)
		}

		if deleted {
			intoBundle.AddDeletedObject(object)
		} else {
			object.SetUID("") // cleanup UID to avoid apply conflict in managed hub
			intoBundle.AddObject(object, objID)
		}
	}

	return timestamp, nil
}

// GetUpdatedManagedClusterLabelsBundles returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects
// belonging to a leaf-hub that had at least once update since the given timestamp, from a specific table.
func (p *PostgreSQL) GetUpdatedManagedClusterLabelsBundles(ctx context.Context, tableName string,
	timestamp *time.Time,
) (map[string]*spec.ManagedClusterLabelsSpecBundle, error) {
	// select ManagedClusterLabelsSpec entries information from DB
	rows, err := p.conn.Query(ctx, fmt.Sprintf(`SELECT leaf_hub_name,managed_cluster_name,labels,
		deleted_label_keys,updated_at,version FROM spec.%[1]s WHERE leaf_hub_name IN (SELECT DISTINCT(leaf_hub_name) 
		from spec.%[1]s WHERE updated_at::timestamp > timestamp '%[2]s') AND leaf_hub_name <> ''`, tableName,
		timestamp.Format(time.RFC3339Nano)))
	if err != nil {
		return nil, fmt.Errorf("failed to query table spec.%s - %w", tableName, err)
	}

	defer rows.Close()

	leafHubToLabelsSpecBundleMap := make(map[string]*spec.ManagedClusterLabelsSpecBundle)

	for rows.Next() {
		var (
			leafHubName              string
			managedClusterLabelsSpec spec.ManagedClusterLabelsSpec
		)

		if err := rows.Scan(&leafHubName, &managedClusterLabelsSpec.ClusterName, &managedClusterLabelsSpec.Labels,
			&managedClusterLabelsSpec.DeletedLabelKeys,
			&managedClusterLabelsSpec.UpdateTimestamp,
			&managedClusterLabelsSpec.Version); err != nil {
			return nil, fmt.Errorf("error reading from table - %w", err)
		}

		// create ManagedClusterLabelsSpecBundle if not mapped for leafHub
		managedClusterLabelsSpecBundle, found := leafHubToLabelsSpecBundleMap[leafHubName]
		if !found {
			managedClusterLabelsSpecBundle = &spec.ManagedClusterLabelsSpecBundle{
				Objects:     []*spec.ManagedClusterLabelsSpec{},
				LeafHubName: leafHubName,
			}

			leafHubToLabelsSpecBundleMap[leafHubName] = managedClusterLabelsSpecBundle
		}

		// append entry to bundle
		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects,
			&managedClusterLabelsSpec)
	}

	return leafHubToLabelsSpecBundleMap, nil
}

// GetEntriesWithDeletedLabels returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects that have a
// none-empty deleted-label-keys column.
func (p *PostgreSQL) GetEntriesWithDeletedLabels(ctx context.Context,
	tableName string,
) (map[string]*spec.ManagedClusterLabelsSpecBundle, error) {
	rows, err := p.conn.Query(ctx, fmt.Sprintf(`SELECT leaf_hub_name,managed_cluster_name,deleted_label_keys,version 
		FROM spec.%s WHERE deleted_label_keys != '[]' AND leaf_hub_name <> ''`, tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to query table spec.%s - %w", tableName, err)
	}

	defer rows.Close()

	leafHubToLabelsSpecBundleMap := make(map[string]*spec.ManagedClusterLabelsSpecBundle)

	for rows.Next() {
		var (
			leafHubName              string
			managedClusterLabelsSpec spec.ManagedClusterLabelsSpec
		)

		if err := rows.Scan(&leafHubName, &managedClusterLabelsSpec.ClusterName,
			&managedClusterLabelsSpec.DeletedLabelKeys, &managedClusterLabelsSpec.Version); err != nil {
			return nil, fmt.Errorf("error reading from table - %w", err)
		}

		// create ManagedClusterLabelsSpecBundle if not mapped for leafHub
		managedClusterLabelsSpecBundle, found := leafHubToLabelsSpecBundleMap[leafHubName]
		if !found {
			managedClusterLabelsSpecBundle = &spec.ManagedClusterLabelsSpecBundle{
				Objects:     []*spec.ManagedClusterLabelsSpec{},
				LeafHubName: leafHubName,
			}

			leafHubToLabelsSpecBundleMap[leafHubName] = managedClusterLabelsSpecBundle
		}

		// append entry to bundle
		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects,
			&managedClusterLabelsSpec)
	}

	return leafHubToLabelsSpecBundleMap, nil
}

// UpdateDeletedLabelKeys updates deleted_label_keys value for a managed cluster entry under
// optimistic concurrency approach.
func (p *PostgreSQL) UpdateDeletedLabelKeys(ctx context.Context, tableName string, readVersion int64,
	leafHubName string, managedClusterName string, deletedLabelKeys []string,
) error {
	deletedLabelsJSON, err := json.Marshal(deletedLabelKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal deleted labels - %w", err)
	}

	if commandTag, err := p.conn.Exec(ctx, fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),deleted_label_keys=$1,
		version=$2 WHERE leaf_hub_name=$3 AND managed_cluster_name=$4 AND version=$5`, tableName), deletedLabelsJSON,
		readVersion+1, leafHubName, managedClusterName, readVersion); err != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", tableName, err)
	} else if commandTag.RowsAffected() == 0 {
		return errOptimisticConcurrencyUpdateFailed
	}

	return nil
}

// GetEntriesWithoutLeafHubName returns a slice of ManagedClusterLabelsSpec that are missing leaf hub name.
func (p *PostgreSQL) GetEntriesWithoutLeafHubName(ctx context.Context,
	tableName string,
) ([]*spec.ManagedClusterLabelsSpec, error) {
	rows, err := p.conn.Query(ctx, fmt.Sprintf(`SELECT managed_cluster_name, version FROM spec.%s WHERE 
		leaf_hub_name = ''`, tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to read from spec.%s - %w", tableName, err)
	}

	defer rows.Close()

	managedClusterLabelsSpecSlice := make([]*spec.ManagedClusterLabelsSpec, 0)

	for rows.Next() {
		var (
			managedClusterName string
			version            int64
		)

		if err := rows.Scan(&managedClusterName, &version); err != nil {
			return nil, fmt.Errorf("error reading from spec.%s - %w", tableName, err)
		}

		managedClusterLabelsSpecSlice = append(managedClusterLabelsSpecSlice, &spec.ManagedClusterLabelsSpec{
			ClusterName: managedClusterName,
			Version:     version,
		})
	}

	return managedClusterLabelsSpecSlice, nil
}

// UpdateLeafHubName updates leaf hub name for a given managed cluster under optimistic concurrency.
func (p *PostgreSQL) UpdateLeafHubName(ctx context.Context, tableName string, readVersion int64,
	managedClusterName string, leafHubName string,
) error {
	if commandTag, err := p.conn.Exec(ctx, fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),leaf_hub_name=$1,version=$2 
		WHERE managed_cluster_name=$3 AND version=$4`, tableName), leafHubName, readVersion+1,
		managedClusterName, readVersion); err != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", tableName, err)
	} else if commandTag.RowsAffected() == 0 {
		return errOptimisticConcurrencyUpdateFailed
	}

	return nil
}

// GetManagedClusterLabelsStatus gets the labels present in managed-cluster CR metadata from a specific table.
func (p *PostgreSQL) GetManagedClusterLabelsStatus(ctx context.Context, tableName string, leafHubName string,
	managedClusterName string,
) (map[string]string, error) {
	labels := make(map[string]string)

	if err := p.conn.QueryRow(ctx, fmt.Sprintf(`SELECT payload->'metadata'->'labels' FROM status.%s WHERE 
		leaf_hub_name=$1 AND payload->'metadata'->>'name'=$2`, tableName), leafHubName,
		managedClusterName).Scan(&labels); err != nil {
		return nil, fmt.Errorf("error reading from table status.%s - %w", tableName, err)
	}

	return labels, nil
}

// GetManagedClusterLeafHubName returns leaf-hub name for a given managed cluster from a specific table.
// TODO: once non-k8s-restapi exposes hub names, remove line.
func (p *PostgreSQL) GetManagedClusterLeafHubName(ctx context.Context, tableName string,
	managedClusterName string,
) (string, error) {
	var leafHubName string
	if err := p.conn.QueryRow(ctx, fmt.Sprintf(`SELECT leaf_hub_name FROM status.%s WHERE 
		payload->'metadata'->>'name'=$1`, tableName), managedClusterName).Scan(&leafHubName); err != nil {
		return "", fmt.Errorf("error reading from table status.%s - %w", tableName, err)
	}

	return leafHubName, nil
}
