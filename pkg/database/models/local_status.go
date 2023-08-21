package models

import "github.com/stolostron/multicluster-global-hub/pkg/database"

type LocalStatusCompliance struct {
	PolicyID    string                    `gorm:"column:policy_id;not null"`
	ClusterName string                    `gorm:"column:cluster_name;not null"`
	LeafHubName string                    `gorm:"column:leaf_hub_name;not null"`
	Error       string                    `gorm:"column:error;not null"`
	Compliance  database.ComplianceStatus `gorm:"column:compliance;not null"`
	ClusterID   string                    `gorm:"column:cluster_id;default:(-)"`
}

func (LocalStatusCompliance) TableName() string {
	return "local_status.compliance"
}
