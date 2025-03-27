package models

import (
	"time"

	"gorm.io/datatypes"
)

type BaseLocalPolicyEvent struct {
	EventName      string         `gorm:"column:event_name;type:varchar(63);not null" json:"eventName"`
	EventNamespace string         `gorm:"column:event_namespace;type:varchar(63);not null" json:"eventNamespace"`
	PolicyID       string         `gorm:"column:policy_id;type:uuid;not null" json:"policyId"`
	Message        string         `gorm:"column:message;type:text" json:"message"`
	LeafHubName    string         `gorm:"size:63;not null" json:"-"`
	Reason         string         `gorm:"column:reason;type:text" json:"reason"`
	Count          int            `gorm:"column:count;type:integer;not null;default:0" json:"count"`
	Source         datatypes.JSON `gorm:"column:source;type:jsonb" json:"source"`
	CreatedAt      time.Time      `gorm:"column:created_at;default:now();not null" json:"createdAt"`
	Compliance     string         `gorm:"column:compliance" json:"compliance"`
}

type LocalReplicatedPolicyEvent struct {
	BaseLocalPolicyEvent
	ClusterID   string `gorm:"column:cluster_id;type:uuid;not null" json:"clusterId"`
	ClusterName string `gorm:"column:cluster_name;type:varchar(63);not null" json:"clusterName"`
}

func (LocalReplicatedPolicyEvent) TableName() string {
	return "event.local_policies"
}

type LocalRootPolicyEvent struct {
	BaseLocalPolicyEvent
}

func (LocalRootPolicyEvent) TableName() string {
	return "event.local_root_policies"
}

type DataRetentionJobLog struct {
	Name         string    `gorm:"column:table_name"`
	StartAt      time.Time `gorm:"column:start_at"`
	EndAt        time.Time `gorm:"column:end_at"`
	MinPartition string    `gorm:"column:min_partition"`
	MaxPartition string    `gorm:"column:max_partition"`
	MinDeletion  time.Time `gorm:"column:min_deletion"`
	Error        string    `gorm:"column:error"`
}

func (DataRetentionJobLog) TableName() string {
	return "event.data_retention_job_log"
}

type ManagedClusterEvent struct {
	EventNamespace      string    `gorm:"column:event_namespace;type:varchar(63);not null" json:"eventNamespace"`
	EventName           string    `gorm:"column:event_name;type:varchar(63);not null" json:"eventName"`
	ClusterName         string    `gorm:"column:cluster_name;type:varchar(63);not null" json:"clusterName"`
	ClusterID           string    `gorm:"column:cluster_id;type:text" json:"clusterId"`
	LeafHubName         string    `gorm:"column:leaf_hub_name;type:varchar(256);not null" json:"leafHubName"`
	Message             string    `gorm:"column:message;type:text" json:"message"`
	Reason              string    `gorm:"column:reason;type:text" json:"reason"`
	ReportingController string    `gorm:"column:reporting_controller;type:text" json:"reportingController"`
	ReportingInstance   string    `gorm:"column:reporting_instance;type:text" json:"reportingInstance"`
	EventType           string    `gorm:"column:event_type;type:varchar(63);not null" json:"type"`
	CreatedAt           time.Time `gorm:"column:created_at;default:now();not null" json:"createdAt"`
}

func (ManagedClusterEvent) TableName() string {
	return "event.managed_clusters"
}

type ClusterGroupUpgradeEvent struct {
	EventNamespace      string         `gorm:"column:event_namespace;type:varchar(63);not null" json:"eventNamespace"`
	EventName           string         `gorm:"column:event_name;type:varchar(63);not null" json:"eventName"`
	EventAnns           datatypes.JSON `gorm:"column:event_annotations;type:jsonb" json:"eventAnnotations,omitempty"`
	CGUName             string         `gorm:"column:cgu_name;type:varchar(63);not null" json:"cguName"`
	LeafHubName         string         `gorm:"column:leaf_hub_name;type:varchar(256);not null" json:"leafHubName"`
	Message             string         `gorm:"column:message;type:text" json:"message"`
	Reason              string         `gorm:"column:reason;type:text" json:"reason"`
	ReportingController string         `gorm:"column:reporting_controller;type:text" json:"reportingController"`
	ReportingInstance   string         `gorm:"column:reporting_instance;type:text" json:"reportingInstance"`
	EventType           string         `gorm:"column:event_type;type:varchar(63);not null" json:"type"`
	CreatedAt           time.Time      `gorm:"column:created_at;default:now();not null" json:"createdAt"`
}

func (ClusterGroupUpgradeEvent) TableName() string {
	return "event.clustergroup_upgrades"
}
