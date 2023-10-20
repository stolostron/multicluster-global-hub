package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var errOptimisticConcurrencyUpdateFailed = errors.New("zero rows were affected by an optimistic concurrency update")

type gormSpecDB struct{}

func NewGormSpecDB() *gormSpecDB {
	return &gormSpecDB{}
}

// GetLastUpdateTimestamp returns the last update timestamp of a specific table.
func (p *gormSpecDB) GetLastUpdateTimestamp(ctx context.Context, tableName string, filterLocalResources bool) (*time.Time, error) {
	db := database.GetGorm()

	var lastTimestamp time.Time
	query := fmt.Sprintf(`SELECT MAX(updated_at) FROM spec.%s WHERE
		payload->'metadata'->'labels'->'global-hub.open-cluster-management.io/global-resource' IS NOT NULL`,
		tableName)

	if !filterLocalResources {
		query = fmt.Sprintf(`SELECT MAX(updated_at) FROM spec.%s`, tableName)
	}

	err := db.Raw(query).Row().Scan(&lastTimestamp)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("no objects in the table spec.%s - %w", tableName, err)
	}

	return &lastTimestamp, nil
}

// QuerySpecObject gets object from given table with object UID
func (p *gormSpecDB) QuerySpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	db := database.GetGorm()

	query := fmt.Sprintf("SELECT payload FROM spec.%s WHERE id = ?", tableName)
	return db.Raw(query, objUID).Row().Scan(&object)
}

// InsertSpecObject insets new object to given table with object UID and payload
func (p *gormSpecDB) InsertSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	db := database.GetGorm()

	query := fmt.Sprintf("INSERT INTO spec.%s (id,payload) values(?, ?)", tableName)
	return db.Exec(query, objUID, object).Error
}

// UpdateSpecObject updates object payload in given table with object UID
func (p *gormSpecDB) UpdateSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	db := database.GetGorm()

	query := fmt.Sprintf("UPDATE spec.%s SET payload = ? WHERE id = ?", tableName)
	return db.Exec(query, object, objUID).Error
}

// DeleteSpecObject deletes object with name and namespace from given table
func (p *gormSpecDB) DeleteSpecObject(ctx context.Context, tableName, name, namespace string) error {
	db := database.GetGorm()

	if namespace != "" {
		query := fmt.Sprintf(`UPDATE spec.%s SET deleted = true WHERE payload -> 'metadata' ->> 'name' = ? AND
			payload -> 'metadata' ->> 'namespace' = ? AND deleted = false`, tableName)
		return db.Exec(query, name, namespace).Error
	} else {
		query := fmt.Sprintf(`UPDATE spec.%s SET deleted = true WHERE payload -> 'metadata' ->> 'name' = ? AND
			payload -> 'metadata' ->> 'namespace' IS NULL AND deleted = false`, tableName)
		return db.Exec(query, name).Error
	}
}

// GetObjectsBundle returns a bundle of objects from a specific table.
func (p *gormSpecDB) GetObjectsBundle(ctx context.Context, tableName string, createObjFunc bundle.CreateObjectFunction,
	intoBundle bundle.ObjectsBundle,
) (*time.Time, error) {
	timestamp, err := p.GetLastUpdateTimestamp(ctx, tableName, true)
	if err != nil {
		return nil, err
	}

	db := database.GetGorm()

	rows, err := db.Raw(fmt.Sprintf(`SELECT id,payload,deleted FROM spec.%s WHERE
		payload->'metadata'->'labels'->'global-hub.open-cluster-management.io/global-resource' IS NOT NULL`,
		tableName)).Rows()
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
func (p *gormSpecDB) GetUpdatedManagedClusterLabelsBundles(ctx context.Context, tableName string,
	timestamp *time.Time,
) (map[string]*spec.ManagedClusterLabelsSpecBundle, error) {
	db := database.GetGorm()
	// select ManagedClusterLabelsSpec entries information from DB
	rows, err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name,managed_cluster_name,labels,
		deleted_label_keys,updated_at,version FROM spec.%[1]s WHERE leaf_hub_name IN (SELECT DISTINCT(leaf_hub_name) 
		from spec.%[1]s WHERE updated_at::timestamp > timestamp '%[2]s') AND leaf_hub_name <> ''`, tableName,
		timestamp.Format(time.RFC3339Nano))).Rows()
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
func (p *gormSpecDB) GetEntriesWithDeletedLabels(ctx context.Context,
	tableName string,
) (map[string]*spec.ManagedClusterLabelsSpecBundle, error) {
	db := database.GetGorm()
	rows, err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name,managed_cluster_name,deleted_label_keys,version 
		FROM spec.%s WHERE deleted_label_keys != '[]' AND leaf_hub_name <> ''`, tableName)).Rows()
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
func (p *gormSpecDB) UpdateDeletedLabelKeys(ctx context.Context, tableName string, readVersion int64,
	leafHubName string, managedClusterName string, deletedLabelKeys []string,
) error {
	deletedLabelsJSON, err := json.Marshal(deletedLabelKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal deleted labels - %w", err)
	}
	db := database.GetGorm()
	if result := db.Exec(fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),deleted_label_keys=?,
		version=? WHERE leaf_hub_name=? AND managed_cluster_name=? AND version=?`, tableName), deletedLabelsJSON,
		readVersion+1, leafHubName, managedClusterName, readVersion); result.Error != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", tableName, err)
	} else if result.RowsAffected == 0 {
		return errOptimisticConcurrencyUpdateFailed
	}
	return nil
}

// GetEntriesWithoutLeafHubName returns a slice of ManagedClusterLabelsSpec that are missing leaf hub name.
func (p *gormSpecDB) GetEntriesWithoutLeafHubName(ctx context.Context, tableName string,
) ([]*spec.ManagedClusterLabelsSpec, error) {
	db := database.GetGorm()

	rows, err := db.Raw(fmt.Sprintf(`SELECT managed_cluster_name, version FROM spec.%s WHERE 
		leaf_hub_name = ''`, tableName)).Rows()
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
func (p *gormSpecDB) UpdateLeafHubName(ctx context.Context, tableName string, readVersion int64,
	managedClusterName string, leafHubName string,
) error {
	db := database.GetGorm()
	if result := db.Exec(fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),leaf_hub_name=?,version=? 
		WHERE managed_cluster_name=? AND version=?`, tableName), leafHubName, readVersion+1,
		managedClusterName, readVersion); result.Error != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", tableName, result.Error)
	} else if result.RowsAffected == 0 {
		return errOptimisticConcurrencyUpdateFailed
	}
	return nil
}

// GetManagedClusterLabelsStatus gets the labels present in managed-cluster CR metadata from a specific table.
func (p *gormSpecDB) GetManagedClusterLabelsStatus(ctx context.Context, tableName string, leafHubName string,
	managedClusterName string,
) (map[string]string, error) {
	labels := make(map[string]string)
	db := database.GetGorm()
	if err := db.Raw(fmt.Sprintf(`SELECT payload->'metadata'->'labels' FROM status.%s WHERE 
		leaf_hub_name=? AND payload->'metadata'->>'name'=?`, tableName), leafHubName,
		managedClusterName).Row().Scan(&labels); err != nil {
		return nil, fmt.Errorf("error reading from table status.%s - %w", tableName, err)
	}
	return labels, nil
}

// GetManagedClusterLeafHubName returns leaf-hub name for a given managed cluster from a specific table.
// TODO: once non-k8s-restapi exposes hub names, remove line.
func (p *gormSpecDB) GetManagedClusterLeafHubName(ctx context.Context, tableName string,
	managedClusterName string,
) (string, error) {
	db := database.GetGorm()
	var leafHubName string
	if err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name FROM status.%s WHERE 
		payload->'metadata'->>'name'=$1`, tableName), managedClusterName).Row().Scan(&leafHubName); err != nil {
		return "", fmt.Errorf("error reading from table status.%s - %w", tableName, err)
	}

	return leafHubName, nil
}
