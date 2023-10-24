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

// CREATE TABLE IF NOT EXISTS spec.policies (
// 	id uuid PRIMARY KEY,
// 	payload jsonb NOT NULL,
// 	created_at timestamp without time zone DEFAULT now() NOT NULL,
// 	updated_at timestamp without time zone DEFAULT now() NOT NULL,
// 	deleted boolean DEFAULT false NOT NULL
// );

type SpecPolicy struct {
	ID        string         `gorm:"column:id;primaryKey"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	Deleted   bool           `gorm:"column:deleted"`
}

func (SpecPolicy) TableName() string {
	return "spec.policies"
}

type SpecPlacementRule struct {
	ID        string         `gorm:"column:id;primaryKey"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	Deleted   bool           `gorm:"column:deleted"`
}

func (SpecPlacementRule) TableName() string {
	return "spec.placementrules"
}

type SpecPlacementBinding struct {
	ID        string         `gorm:"column:id;primaryKey"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	Deleted   bool           `gorm:"column:deleted"`
}

func (SpecPlacementBinding) TableName() string {
	return "spec.placementbindings"
}
