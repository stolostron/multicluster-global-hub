package conflator

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

// EventHandleFunc is a function for handling a bundle.
type EventHandleFunc func(context.Context, *cloudevents.Event) error

// ConflationRegistration is used to register a new conflated bundle type along with its priority and handler function.
type ConflationRegistration struct {
	priority   ConflationPriority
	syncMode   enum.EventSyncMode
	eventType  string
	handleFunc EventHandleFunc
	dependency *dependency.Dependency
}

// NewConflationRegistration creates a new instance of ConflationRegistration.
func NewConflationRegistration(priority ConflationPriority, syncMode enum.EventSyncMode, eventType string,
	handlerFunction EventHandleFunc,
) *ConflationRegistration {
	return &ConflationRegistration{
		priority:   priority,
		syncMode:   syncMode,
		eventType:  eventType,
		handleFunc: handlerFunction,
		dependency: nil,
	}
}

// WithDependency declares a dependency required by the given bundle type.
func (registration *ConflationRegistration) WithDependency(val *dependency.Dependency) *ConflationRegistration {
	registration.dependency = val
	return registration
}
