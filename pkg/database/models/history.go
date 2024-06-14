package models

import "time"

type LocalComplianceJobLog struct {
	Name     string    `gorm:"column:name"`
	StartAt  time.Time `gorm:"column:start_at;default:current_timestamp"`
	EndAt    time.Time `gorm:"column:end_at;default:current_timestamp"`
	Total    int64     `gorm:"column:total"`
	Inserted int64     `gorm:"column:inserted"`
	Offsets  int64     `gorm:"column:offsets"`
	Error    string    `gorm:"column:error"`
}

func (LocalComplianceJobLog) TableName() string {
	return "history.local_compliance_job_log"
}

type LocalComplianceHistory struct {
	PolicyID                   string    `gorm:"column:policy_id"`
	ClusterID                  string    `gorm:"olumn:cluster_id"`
	LeafHubName                string    `gorm:"type:varchar(254);not null;column:leaf_hub_name"`
	ComplianceDate             time.Time `gorm:"type:date;column:compliance_date"`
	Compliance                 string    `gorm:"type:compliance_type;not null;column:compliance"`
	ComplianceChangedFrequency int       `gorm:"type:integer;column:compliance_changed_frequency"`
}

// TableName specifies the table name for the LocalCompliance model
func (LocalComplianceHistory) TableName() string {
	return "history.local_compliance"
}
