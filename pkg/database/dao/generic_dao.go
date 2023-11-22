package dao

import (
	"encoding/json"
	"fmt"

	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"gorm.io/gorm"
)

type GenericDao struct {
	tx    *gorm.DB
	table string
}

func NewGenericDao(tx *gorm.DB, tableName string) *GenericDao {
	return &GenericDao{
		tx:    tx,
		table: tableName,
	}
}

// GetIdToVersionByHub returns a map for resourceId to resourceVersion from a leaf hub
func (dao *GenericDao) GetIdToVersionByHub(hubName string) (map[string]string, error) {
	sqlTemplate := fmt.Sprintf(
		`SELECT 
			id AS key, 
			payload->'metadata'->>'resourceVersion AS resource_version' 
		FROM 
			%s 
		WHERE 
			leaf_hub_name = ?`, dao.table)

	var resourceVersions []models.ResourceVersion
	err := dao.tx.Raw(sqlTemplate, hubName).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}

	idToVersionMap := make(map[string]string)
	for _, resource := range resourceVersions {
		idToVersionMap[resource.Key] = resource.ResourceVersion
	}
	return idToVersionMap, nil
}

func (dao *GenericDao) Insert(hubName, id string, obj interface{}) error {
	sqlTemplate := fmt.Sprintf(
		`INSERT INTO %s (id, leaf_hub_name, payload) VALUES ($1::uuid, $2, $3::jsonb)`, dao.table)

	payload, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return dao.tx.Exec(sqlTemplate, id, hubName, payload).Error
}

func (dao *GenericDao) Update(hubName, id string, obj interface{}) error {
	sqlTemplate := fmt.Sprintf(
		`UPDATE %s SET payload = $1::jsonb WHERE leaf_hub_name = $2 AND id = $3::uuid`, dao.table)

	payload, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return dao.tx.Exec(sqlTemplate, payload, hubName, id).Error
}

func (dao *GenericDao) Delete(hubName, id string) error {
	sqlTemplate := fmt.Sprintf(
		`DELETE FROM %s WHERE leaf_hub_name = $1 AND id = $2::uuid`, dao.table)
	return dao.tx.Exec(sqlTemplate, hubName, id).Error
}
