package handler

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type EventHandler interface {
	EventType() enum.EventType
	ToDatabase(evt cloudevents.Event) error
}
