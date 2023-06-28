package models

import (
	"time"

	"gorm.io/datatypes"
)

type BaseLocalPolicyEvent struct {
	EventName   string         `gorm:"column:event_name;type:varchar(63);not null"`
	PolicyID    string         `gorm:"column:policy_id;type:uuid;not null"`
	Message     string         `gorm:"column:message;type:text"`
	LeafHubName string         `gorm:"size:63;not null"`
	Reason      string         `gorm:"column:reason;type:text"`
	Count       int            `gorm:"column:count;type:integer;not null;default:0"`
	Source      datatypes.JSON `gorm:"column:source;type:jsonb"`
	CreatedAt   time.Time      `gorm:"column:created_at;default:now();not null"`
	Compliance  string         `gorm:"column:compliance"`
}

type LocalClusterPolicyEvent struct {
	BaseLocalPolicyEvent
	ClusterID string `gorm:"column:cluster_id;type:uuid;not null"`
}

func (LocalClusterPolicyEvent) TableName() string {
	return "event.local_policies"
}

type LocalRootPolicyEvent struct {
	BaseLocalPolicyEvent
}

func (LocalRootPolicyEvent) TableName() string {
	return "event.local_root_policies"
}
