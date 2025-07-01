package emitters

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Emitter defines methods to manage bundle lifecycle, including object updates,
// size validation, and version control. All methods are not safe for concurrent use
// and must be externally synchronized if used in parallel.
type Emitter interface {
	// EventType returns the type of event this emitter will handle.
	EventType() string

	// EventFilter returns the predicate used to filter events.
	EventFilter() predicate.Predicate

	// Update modifies the bundle with the provided object.
	// Returns an error if the update fails.
	Update(obj client.Object) error

	// Delete removes the bundle associated with the provided object.
	// Returns an error if the deletion fails.

	Delete(obj client.Object) error

	// Resync periodically reconciles the state of the provided objects.
	// Returns an error if the resynchronization fails.
	Resync(objects []client.Object) error

	// Send triggers the emission of an event.
	// Returns an error if sending fails.
	Send() error
}
