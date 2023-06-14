package models

import (
	"time"

	"gorm.io/datatypes"
)

type ManagedCluster struct {
	LeafHubName string         `gorm:"column:leaf_hub_name;not null"`
	ClusterID   string         `gorm:"column:cluster_id;not null"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Error       string         `gorm:"column:error;not null"`
	ClusterName string         `gorm:"column:cluster_name;default:(-)"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:(-)"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;default:(-)"`
	DeletedAt   time.Time      `gorm:"column:deleted_at;default:(-)"`
}

func (ManagedCluster) TableName() string {
	return "status.managed_clusters"
}
