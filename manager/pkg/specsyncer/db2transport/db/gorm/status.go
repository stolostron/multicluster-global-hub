package gorm

import (
	"context"
	"fmt"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

type gormStatusDB struct{}

func NewGormStausDB() *gormStatusDB {
	return &gormStatusDB{}
}

// GetManagedClusterLabelsStatus gets the labels present in managed-cluster CR metadata from a specific table.
func (p *gormStatusDB) GetManagedClusterLabelsStatus(ctx context.Context, tableName string, leafHubName string,
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
func (p *gormStatusDB) GetManagedClusterLeafHubName(ctx context.Context, tableName string,
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
