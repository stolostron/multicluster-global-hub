package conflator

import (
	"context"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
)

// BundleHandlerFunc is a function for handling a bundle.
type BundleHandlerFunc func(context.Context, bundle.ManagerBundle) error

// ConflationRegistration is used to register a new conflated bundle type along with its priority and handler function.
type ConflationRegistration struct {
	priority        ConflationPriority
	syncMode        metadata.BundleSyncMode
	bundleType      string
	handlerFunction BundleHandlerFunc
	dependency      *dependency.Dependency
}

// NewConflationRegistration creates a new instance of ConflationRegistration.
func NewConflationRegistration(priority ConflationPriority, syncMode metadata.BundleSyncMode, bundleType string,
	handlerFunction BundleHandlerFunc,
) *ConflationRegistration {
	return &ConflationRegistration{
		priority:        priority,
		syncMode:        syncMode,
		bundleType:      bundleType,
		handlerFunction: handlerFunction,
		dependency:      nil,
	}
}

// WithDependency declares a dependency required by the given bundle type.
func (registration *ConflationRegistration) WithDependency(dependency *dependency.Dependency) *ConflationRegistration {
	registration.dependency = dependency
	return registration
}
