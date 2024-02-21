package generic

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Object is an interface for a single object inside a bundle.
type ObjectController interface {
	Instance() client.Object
	Predicate() predicate.Predicate
	EnableFinalizer() bool
}

type EventEmitter interface {
	// covert the current payload to a cloudevent
	ToCloudEvent() *cloudevents.Event

	// topic
	Topic() string

	// to assert whether send the current cloudevent
	PreSend() bool

	// triggered after sending the event, incr generate, clean payload, ...
	PostSend()
}

type MultiEventEmitter interface {
	EventEmitter

	// assert whether to get into the emitter
	Predicate(client.Object) bool

	// to update the cloudevent payload by the updated object
	Update(client.Object)

	// to update the cloudevent payload by the deleting object
	Delete(client.Object)
}
