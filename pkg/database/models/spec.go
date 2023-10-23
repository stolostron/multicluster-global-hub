package models

import (
	"time"

	"gorm.io/datatypes"
)

type ManagedClusterLabel struct {
	ID                 string         `gorm:"column:id;primaryKey"`
	LeafHubName        string         `gorm:"column:leaf_hub_name;not null"`
	ManagedClusterName string         `gorm:"column:managed_cluster_name;not null"`
	Labels             datatypes.JSON `gorm:"column:labels;type:jsonb"`
	DeletedLabelKeys   datatypes.JSON `gorm:"column:deleted_label_keys;type:jsonb"`
	Version            int            `gorm:"column:version;not null"`
	UpdatedAt          time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	// CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:true"`
	// DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (ManagedClusterLabel) TableName() string {
	return "spec.managed_clusters_labels"
}
