package bundle

import (
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	statusbundle "github.com/stolostron/multicluster-globalhub/pkg/bundle/status"
)

// ErrObjectNotFound error to be used when an object is not found.
var ErrObjectNotFound = errors.New("object not found")

// ExtractObjIDFunc a function type used to get the id of an object.
type ExtractObjIDFunc func(obj Object) (string, bool)

// Object is an interface for a single object inside a bundle.
type Object interface {
	metav1.Object
	runtime.Object
}

// Bundle is an abstraction for managing different bundle types.
type Bundle interface {
	// UpdateObject function to update a single object inside a bundle.
	UpdateObject(object Object)
	// DeleteObject function to delete a single object inside a bundle.
	DeleteObject(object Object)
	// GetBundleVersion function to get bundle generation.
	GetBundleVersion() *statusbundle.BundleVersion
}

// DeltaStateBundle abstracts the logic needed from the delta-state bundle.
type DeltaStateBundle interface {
	Bundle
	// GetTransportationID function to get bundle transportation ID to be attached to message-key during transportation.
	GetTransportationID() int
	// SyncState syncs the state of the delta-bundle with the full-state.
	SyncState()
	// Reset flushes the delta-state bundle's objects.
	Reset()
}
