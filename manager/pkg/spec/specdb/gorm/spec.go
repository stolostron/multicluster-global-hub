package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

var errQueryTableFailedTemplate = "failed to query table spec.%s - %w"

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
	conn := database.GetConn()

	err := database.Lock(conn)
	if err != nil {
		return err
	}
	defer database.Unlock(conn)
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
	conn := database.GetConn()

	err := database.Lock(conn)
	if err != nil {
		return err
	}
	defer database.Unlock(conn)
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
	conn := database.GetConn()

	err := database.Lock(conn)
	if err != nil {
		return err
	}
	defer database.Unlock(conn)
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

	defer func() { _ = rows.Close() }()

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
