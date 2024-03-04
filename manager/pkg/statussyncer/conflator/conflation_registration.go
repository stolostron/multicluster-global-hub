package conflator

import (
	"context"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// EventHandleFunc is a function for handling a bundle.
type EventHandleFunc func(context.Context, *cloudevents.Event) error

// ConflationRegistration is used to register a new conflated bundle type along with its priority and handler function.
type ConflationRegistration struct {
	priority   ConflationPriority
	syncMode   metadata.EventSyncMode
	eventType  string
	handleFunc EventHandleFunc
	dependency *Dependency
}

// NewConflationRegistration creates a new instance of ConflationRegistration.
func NewConflationRegistration(priority ConflationPriority, syncMode metadata.EventSyncMode, eventType string,
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
func (registration *ConflationRegistration) WithDependency(dependency *Dependency) *ConflationRegistration {
	registration.dependency = dependency
	return registration
}

// NewDependency creates a new instance of dependency.
func NewDependency(eventType string, dependencyType dependencyType) *Dependency {
	return &Dependency{
		EventType:      eventType,
		DependencyType: dependencyType,
	}
}

// Dependency represents the dependency between different bundles. a bundle can depend only on one other bundle.
type Dependency struct {
	EventType      string
	DependencyType dependencyType
}

type dependencyType string

const (
	// ExactMatch used to specify that dependant bundle requires the exact version of the dependency to be the
	// last processed bundle.
	ExactMatch dependencyType = "ExactMatch"
	// AtLeast used to specify that dependant bundle requires at least some version of the dependency to be processed.
	AtLeast dependencyType = "AtLeast"
)
