package event

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

type BaseEvent struct {
	EventName      string             `json:"eventName"`
	EventNamespace string             `json:"eventNamespace"`
	Message        string             `json:"message,omitempty"`
	Reason         string             `json:"reason,omitempty"`
	Count          int32              `json:"count,omitempty"`
	Source         corev1.EventSource `json:"source,omitempty"`
	CreatedAt      time.Time          `json:"createdAt,omitempty"`
}
