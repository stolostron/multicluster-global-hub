package conflator

import (
	"context"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/dependency"
)

// BundleHandlerFunc is a function for handling a bundle.
type BundleHandlerFunc func(context.Context, statusbundle.Bundle, db.StatusTransportBridgeDB) error

// NewConflationRegistration creates a new instance of ConflationRegistration.
func NewConflationRegistration(priority ConflationPriority, syncMode bundle.BundleSyncMode, bundleType string,
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

// ConflationRegistration is used to register a new conflated bundle type along with its priority and handler function.
type ConflationRegistration struct {
	priority        ConflationPriority
	syncMode        bundle.BundleSyncMode
	bundleType      string
	handlerFunction BundleHandlerFunc
	dependency      *dependency.Dependency
}

// WithDependency declares a dependency required by the given bundle type.
func (registration *ConflationRegistration) WithDependency(dependency *dependency.Dependency) *ConflationRegistration {
	registration.dependency = dependency
	return registration
}
