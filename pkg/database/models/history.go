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
