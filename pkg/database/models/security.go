package models

import "time"

// SecurityAlertCounts contains a summary of the security alerts from a hub.
type SecurityAlertCounts struct {
	// HubName is the name of the hub.
	HubName string `gorm:"column:hub_name;primaryKey"`

	// Low is the total number of low severity alerts.
	Low int `gorm:"column:low;not null"`

	// Medium is the total number of medium severity alerts.
	Medium int `gorm:"column:medium;not null"`

	// High is the total number of high severity alerts.
	High int `gorm:"column:high;not null"`

	// Critical is the total number of critical severity alerts.
	Critical int `gorm:"column:critical;not null"`

	// DetailURL is the URL where the user can see the details of the alerts of the hub. This
	// will typically be the URL of the violations tab of the Stackrox Central UI:
	//
	//	https://central-rhacs-operator.apps.../main/violations
	DetailURL string `gorm:"column:detail_url;not null"`

	// Source is the Central CR instance from which the data was retrieved.
	// This should follow the format: "<namespace>/<name>"
	Source string `gorm:"column:source;not null"`

	// CreatedAt is the date and time when the row was created.
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime:true"`

	// UpdatedAt is the date and time when the row was last updated.
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime:true"`
}

func (SecurityAlertCounts) TableName() string {
	return "security.alert_counts"
}
