package models

type ResourceVersion struct {
	Key             string `gorm:"column:key"`
	ResourceVersion string `gorm:"column:resource_version"`
}
