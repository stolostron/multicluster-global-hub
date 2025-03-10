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
	CreatedAt   time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (LeafHub) TableName() string {
	return "status.leaf_hubs"
}

type StatusCompliance struct {
	PolicyID    string                    `gorm:"column:policy_id;primaryKey"`
	ClusterName string                    `gorm:"column:cluster_name;primaryKey"`
	LeafHubName string                    `gorm:"column:leaf_hub_name;primaryKey"`
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

type Transport struct {
	Name      string         `gorm:"column:name;primaryKey"`
	Payload   datatypes.JSON `gorm:"column:payload;type:jsonb"` // KafkaPosition
	CreatedAt time.Time      `gorm:"autoCreateTime:true"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime:true"`
}

func (Transport) TableName() string {
	return "status.transport"
}

type LeafHubHeartbeat struct {
	Name         string    `gorm:"column:leaf_hub_name;primaryKey"`
	Status       string    `gorm:"column:status;default:(-)"`
	LastUpdateAt time.Time `gorm:"column:last_timestamp;autoUpdateTime:false"`
}

func (LeafHubHeartbeat) TableName() string {
	return "status.leaf_hub_heartbeats"
}

func (h LeafHubHeartbeat) UpInsertHeartBeat(db *gorm.DB) error {
	tmp := `INSERT INTO status.leaf_hub_heartbeats (leaf_hub_name, status, last_timestamp)
		VALUES ($1, $2, $3) ON CONFLICT (leaf_hub_name) DO UPDATE SET last_timestamp = $3;`
	return db.Exec(tmp, h.Name, h.Status, h.LastUpdateAt).Error
}

type SubscriptionReport struct {
	ID          string         `gorm:"column:id;primaryKey"`
	LeafHubName string         `gorm:"type:varchar(254);column:leaf_hub_name"`
	Payload     datatypes.JSON `gorm:"type:jsonb;column:payload"`
}

// TableName specifies the table name for the SubscriptionReport model
func (SubscriptionReport) TableName() string {
	return "status.subscription_reports"
}

type ManagedClusterMigration struct {
	ID          uint           `gorm:"primaryKey;autoIncrement"`
	FromHub     string         `gorm:"not null;index"`
	ToHub       string         `gorm:"not null;index"`
	ClusterName string         `gorm:"not null;index"`
	Payload     datatypes.JSON `gorm:"type:jsonb"`
	Stage       string         `gorm:"type:text;not null;check:stage IN ('initializing', 'validating', 'registering', 'deploying', 'completed')"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
}

func (ManagedClusterMigration) TableName() string {
	return "status.managed_cluster_migration"
}
