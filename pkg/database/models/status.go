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
	ClusterName string         `gorm:"-"`
	CreatedAt   time.Time      `gorm:"-"`
	UpdatedAt   time.Time      `gorm:"-"`
	DeletedAt   *time.Time     `gorm:"-"`
}

func (ManagedCluster) TableName() string {
	return "status.managed_clusters"
}
