package generic

import (
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Object is an interface for a single object inside a bundle.
type ObjectSyncer interface {
	Name() string
	Instance() client.Object
	Predicate() predicate.Predicate
	Interval() func() time.Duration
	EnableFinalizer() bool
}

type EventEmitter interface {
	// to update the cloudevent payload by the updated object
	Update(client.Object)
	// to update the cloudevent payload by the deleting object
	Delete(client.Object)

	// covert the current payload to a cloudevent
	ToCloudEvent() *cloudevents.Event

	// to assert whether emit the current cloudevent
	Emit() bool

	// topic
	Topic() string

	// triggered after sending the event, incr generate, clean payload, ...
	PostSend()
}
