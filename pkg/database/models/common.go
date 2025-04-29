package models

import "time"

type ResourceVersion struct {
	Key             string `gorm:"column:key"`
	Name            string `gorm:"column:name"`
	ResourceVersion string `gorm:"column:resource_version"`
}

type ClusterInfo struct {
	ConsoleURL string `gorm:"column:console_url"`
	MchVersion string `gorm:"column:mch_version"`
}

type Table struct {
	Schema string `gorm:"column:schema_name"`
	Table  string `gorm:"column:table_name"`
}

// This is for time column from database
type Time struct {
	Time time.Time `gorm:"column:time"`
}
