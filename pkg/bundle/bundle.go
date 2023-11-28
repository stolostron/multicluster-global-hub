package bundle

import (
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
)

// Object is an interface for a single object inside a bundle.
type Object interface {
	metav1.Object
	runtime.Object
}

type Bundle interface {
	AgentBundle
	ManagerBundle
}

// BundleHandler is an abstraction for managing different object for a bundle.
type ObjectHandler interface {
	Predicate() predicate.Predicate
	CreateObject() Object
	SyncIntervalFunc() func() time.Duration
	BundleUpdate(obj Object, b BaseAgentBundle)
	BundleDelete(obj Object, b BaseAgentBundle)
}

type BaseAgentBundle interface {
	// GetVersion function to get bundle generation.
	GetVersion() *metadata.BundleVersion
}

// AgentBundle is an abstraction for managing different bundle types.
type AgentBundle interface {
	BaseAgentBundle
	// UpdateObject function to update a single object inside a bundle.
	UpdateObject(object Object)
	// DeleteObject function to delete a single object inside a bundle.
	DeleteObject(object Object)
}

// AgentDeltaBundle abstracts the logic needed from the delta-state bundle.
type AgentDeltaBundle interface {
	AgentBundle
	// GetTransportationID function to get bundle transportation ID to be attached to message-key during transportation.
	GetTransportationID() int
	// SyncState syncs the state of the delta-bundle with the full-state.
	SyncState()
	// Reset flushes the delta-state bundle's objects.
	Reset()
}

// ManagerBundle bundles together a set of objects that were sent from leaf hubs via transport layer.
type ManagerBundle interface {
	// GetLeafHubName returns the leaf hub name that sent the bundle.
	GetLeafHubName() string
	// GetObjects returns the objects in the bundle.
	GetObjects() []interface{}
	// GetVersion returns the bundle version.
	GetVersion() *metadata.BundleVersion
	// SetVersion sets the bundle version. ref to https://github.com/stolostron/multicluster-global-hub/pull/563
	SetVersion(version *metadata.BundleVersion)
}

// ManagerDependantBundle is a bundle that depends on a different bundle.
// to support bundles dependencies additional function is required - GetDependencyVersion, in order to start
// processing the dependant bundle only after its required dependency (with required version) was processed.
type ManagerDependantBundle interface {
	ManagerBundle
	// GetDependencyVersion returns the bundle dependency required version.
	GetDependencyVersion() *metadata.BundleVersion
}

// ManagerDeltaBundle abstracts the functionality required from a Bundle to be used as Delta-State bundle.
type ManagerDeltaBundle interface {
	ManagerDependantBundle
	// InheritEvents inherits the events in an older delta-bundle into the receiver (in-case of conflict, the receiver
	// is the source of truth).
	InheritEvents(olderBundle ManagerBundle) error
}

// GetBundleType returns the concrete type of a bundle.
func GetBundleType(bundle ManagerBundle) string {
	array := strings.Split(fmt.Sprintf("%T", bundle), ".")
	return array[len(array)-1]
}
