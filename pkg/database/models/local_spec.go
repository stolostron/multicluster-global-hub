package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type LocalSpecPolicy struct {
	LeafHubName    string         `gorm:"column:leaf_hub_name"`
	Payload        datatypes.JSON `gorm:"column:payload;type:jsonb"`
	PolicyID       string         `gorm:"column:policy_id;primaryKey"`
	PolicyName     string         `gorm:"column:policy_name;default:(-)"`
	PolicyStandard string         `gorm:"column:policy_standard;default:(-)"`
	PolicyCategory string         `gorm:"column:policy_category;default:(-)"`
	PolicyControl  string         `gorm:"column:policy_control;default:(-)"`
	CreatedAt      time.Time      `gorm:"column:created_at;autoCreateTime:true"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;autoUpdateTime:true"`
	DeletedAt      gorm.DeletedAt `gorm:"column:deleted_at"`
}

func (LocalSpecPolicy) TableName() string {
	return "local_spec.policies"
}
