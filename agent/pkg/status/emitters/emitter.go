package emitters

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Emitter defines methods to manage bundle lifecycle, including object updates,
// size validation, and version control. All methods are not safe for concurrent use
// and must be externally synchronized if used in parallel.
type Emitter interface {
	EventType() string
	EventFilter() predicate.Predicate
	// Update and Delete to modify bundle in controller
	Update(obj client.Object) error
	Delete(obj client.Object) error
	// Resync and Send is invoked in the periodic controller
	Resync(objects []client.Object) error
	Send() error
}
