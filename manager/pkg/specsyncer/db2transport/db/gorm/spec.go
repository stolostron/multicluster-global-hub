package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v4"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var (
	errOptimisticConcurrencyUpdateFailed = errors.New("zero rows were affected by an optimistic concurrency update")
	errQueryTableFailedTemplate          = "failed to query table spec.%s - %w"
)

type gormSpecDB struct{}

func NewGormSpecDB() *gormSpecDB {
	return &gormSpecDB{}
}

// GetLastUpdateTimestamp returns the last update timestamp of a specific table.
func (p *gormSpecDB) GetLastUpdateTimestamp(ctx context.Context, tableName string,
	filterLocalResources bool,
) (*time.Time, error) {
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
	var payload []byte
	query := fmt.Sprintf("SELECT payload FROM spec.%s WHERE id = ?", tableName)
	err := db.Raw(query, objUID).Row().Scan(&payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, &object)
}

// InsertSpecObject insets new object to given table with object UID and payload
func (p *gormSpecDB) InsertSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	db := database.GetGorm()
	query := fmt.Sprintf("INSERT INTO spec.%s (id, payload) values(?, ?)", tableName)
	payload, err := json.Marshal(object)
	if err != nil {
		return err
	}
	return db.Exec(query, objUID, payload).Error
}

// UpdateSpecObject updates object payload in given table with object UID
func (p *gormSpecDB) UpdateSpecObject(ctx context.Context, tableName, objUID string, object *client.Object) error {
	db := database.GetGorm()
	payload, err := json.Marshal(object)
	if err != nil {
		return err
	}
	query := fmt.Sprintf("UPDATE spec.%s SET payload = ? WHERE id = ?", tableName)
	return db.Exec(query, payload, objUID).Error
}

// DeleteSpecObject deletes object with name and namespace from given table
func (p *gormSpecDB) DeleteSpecObject(ctx context.Context, tableName, name, namespace string) error {
	db := database.GetGorm()
	updateTemplate := fmt.Sprintf(`UPDATE spec.%s SET deleted = true WHERE payload -> 'metadata' ->> 'name' = '%s' AND
	deleted = false`, tableName, name)
	namespaceCondition := " AND payload -> 'metadata' ->> 'namespace' IS NULL"
	if namespace != "" {
		namespaceCondition = fmt.Sprintf(` AND payload -> 'metadata' ->> 'namespace' = '%s'`, namespace)
	}
	return db.Exec(updateTemplate + namespaceCondition).Error
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
		return nil, fmt.Errorf(errQueryTableFailedTemplate, tableName, err)
	}

	defer rows.Close()

	for rows.Next() {
		var (
			objID   string
			deleted bool
		)
		object := createObjFunc()

		var payload []byte
		if err := rows.Scan(&objID, &payload, &deleted); err != nil {
			return nil, fmt.Errorf("error reading from table spec.%s - %w", tableName, err)
		}
		if err := json.Unmarshal(payload, &object); err != nil {
			return nil, fmt.Errorf("error reading unmarshal payload from table spec.%s - %w", tableName, err)
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
	rows, err := db.Raw(fmt.Sprintf(`SELECT * FROM spec.%[1]s WHERE leaf_hub_name IN (SELECT DISTINCT(leaf_hub_name) 
		from spec.%[1]s WHERE updated_at::timestamp > timestamp '%[2]s') AND leaf_hub_name <> ''`, tableName,
		timestamp.Format(time.RFC3339Nano))).Rows()
	if err != nil {
		return nil, fmt.Errorf(errQueryTableFailedTemplate, tableName, err)
	}
	defer rows.Close()

	leafHubToLabelsSpecBundleMap := make(map[string]*spec.ManagedClusterLabelsSpecBundle)

	for rows.Next() {
		// var leafHubName string // managedClusterLabelsSpec spec.ManagedClusterLabelsSpec
		var managedClusterLabel models.ManagedClusterLabel
		if err := db.ScanRows(rows, &managedClusterLabel); err != nil {
			return nil, fmt.Errorf("error reading managed cluster label from table - %w", err)
		}

		// create ManagedClusterLabelsSpecBundle if not mapped for leafHub
		managedClusterLabelsSpecBundle, found := leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName]
		if !found {
			managedClusterLabelsSpecBundle = &spec.ManagedClusterLabelsSpecBundle{
				Objects:     []*spec.ManagedClusterLabelsSpec{},
				LeafHubName: managedClusterLabel.LeafHubName,
			}
			leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName] = managedClusterLabelsSpecBundle
		}

		labels := map[string]string{}
		err := json.Unmarshal(managedClusterLabel.Labels, &labels)
		if err != nil {
			return nil, fmt.Errorf("error to unmarshal labels - %w", err)
		}

		deletedKeys := []string{}
		err = json.Unmarshal(managedClusterLabel.DeletedLabelKeys, &deletedKeys)
		if err != nil {
			return nil, fmt.Errorf("error to unmarshal deletedKeys - %w", err)
		}

		// append entry to bundle
		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects,
			&spec.ManagedClusterLabelsSpec{
				ClusterName:      managedClusterLabel.ManagedClusterName,
				Version:          int64(managedClusterLabel.Version),
				UpdateTimestamp:  managedClusterLabel.UpdatedAt,
				Labels:           labels,
				DeletedLabelKeys: deletedKeys,
			})
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
		return nil, fmt.Errorf(errQueryTableFailedTemplate, tableName, err)
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
