package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type ManagedCluster struct {
	LeafHubName string         `gorm:"column:leaf_hub_name;not null"`
	ClusterID   string         `gorm:"column:cluster_id;not null"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Error       string         `gorm:"column:error;not null"`
	ClusterName string         `gorm:"column:cluster_name;default:(-)"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:(-)"` // https://gorm.io/docs/conventions.html#CreatedAt
	UpdatedAt   time.Time      `gorm:"column:updated_at;default:(-)"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;default:(-)"`
}

func (ManagedCluster) TableName() string {
	return "status.managed_clusters"
}

type LeafHub struct {
	LeafHubName string         `gorm:"column:leaf_hub_name;not null"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	ConsoleURL  string         `gorm:"column:console_url;default:(-)"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:(-)"` // https://gorm.io/docs/conventions.html#CreatedAt
	UpdatedAt   time.Time      `gorm:"column:updated_at;default:(-)"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;default:(-)"`
}

func (LeafHub) TableName() string {
	return "status.leaf_hubs"
}
