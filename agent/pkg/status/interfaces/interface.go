package interfaces

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Object is an interface for a single object inside a bundle.
type Controller interface {
	Instance() client.Object
	Predicate() predicate.Predicate
}

// ObjectController defines methods for managing instances of objects.
type ObjectController interface {
	// Instance returns the current object instance associated with this controller.
	Instance() client.Object

	// List retrieves a list of objects managed by this controller.
	// The parameter 'c' is an instance of the client used for fetching objects.
	List(c client.Client) ([]client.Object, error)
}

// Use the event emitter to control the flow of the event syncer
type Emitter interface {
	PostUpdate()

	ToCloudEvent(data interface{}) (*cloudevents.Event, error)

	// topic
	Topic() string

	// to assert whether send the current cloudevent
	ShouldSend() bool

	// triggered after sending the event, incr generate, clean payload, ...
	PostSend(data interface{})
}

// Use this interface to update the event payload/data by the client.Object
type Handler interface {
	// Get the bundle as the cloudevent data
	Get() interface{}
	// the method is for the controller to update payload by the object
	Update(object client.Object) bool
	// the method is for the controller to update payload by the object
	Delete(object client.Object) bool
}
