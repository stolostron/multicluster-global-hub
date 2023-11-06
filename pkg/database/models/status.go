package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

type ManagedCluster struct {
	LeafHubName string         `gorm:"column:leaf_hub_name;not null"`
	ClusterID   string         `gorm:"column:cluster_id;primaryKey"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	Error       string         `gorm:"column:error;not null"`
	// ClusterName string         `gorm:"column:cluster_name"`
	CreatedAt time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (ManagedCluster) TableName() string {
	return "status.managed_clusters"
}

type LeafHub struct {
	LeafHubName string         `gorm:"column:leaf_hub_name;not null"`
	ClusterID   string         `gorm:"column:cluster_id;primaryKey"`
	Payload     datatypes.JSON `gorm:"column:payload;type:jsonb"`
	ConsoleURL  string         `gorm:"column:console_url;default:(-)"`
	GrafanaURL  string         `gorm:"column:grafana_url;default:(-)"`
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (LeafHub) TableName() string {
	return "status.leaf_hubs"
}

type StatusCompliance struct {
	PolicyID    string                    `gorm:"column:policy_id;not null"`
	ClusterName string                    `gorm:"column:cluster_name;not null"`
	LeafHubName string                    `gorm:"column:leaf_hub_name;not null"`
	Error       string                    `gorm:"column:error;not null"`
	Compliance  database.ComplianceStatus `gorm:"column:compliance;not null"`
	// ClusterID   string                    `gorm:"column:cluster_id;default:(-)"`
}

func (StatusCompliance) TableName() string {
	return "status.compliance"
}

type AggregatedCompliance struct {
	PolicyID             string `gorm:"column:policy_id;not null"`
	LeafHubName          string `gorm:"column:leaf_hub_name;not null"`
	AppliedClusters      int    `gorm:"column:applied_clusters;not null"`
	NonCompliantClusters int    `gorm:"column:non_compliant_clusters;not null"`
}

func (a AggregatedCompliance) TableName() string {
	return "status.aggregated_compliance"
}
