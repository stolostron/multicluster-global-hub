package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

type LocalSpecPolicy struct {
	LeafHubName    string         `gorm:"column:leaf_hub_name"`
	Payload        datatypes.JSON `gorm:"column:payload;type:jsonb"`
	PolicyID       uuid.UUID      `gorm:"-"`
	PolicyName     string         `gorm:"-"`
	PolicyStandard string         `gorm:"-"`
	PolicyCategory string         `gorm:"-"`
	PolicyControl  string         `gorm:"-"`
	CreatedAt      time.Time      `gorm:"-"`
	UpdatedAt      time.Time      `gorm:"-"`
	DeletedAt      time.Time      `gorm:"-"`
}

func (LocalSpecPolicy) TableName() string {
	return "local_spec.policies"
}
