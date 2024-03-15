package generic

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var _ ObjectEmitter = &genericObjectEmitter{}

type genericObjectEmitter struct {
	*genericEmitter
	Handler
}

func NewGenericObjectEmitter(eventType enum.EventType, eventData interface{},
	handler Handler, opts ...EmitterOption,
) ObjectEmitter {
	return &genericObjectEmitter{
		NewGenericEmitter(eventType, eventData, opts...),
		handler,
	}
}

func (e *genericObjectEmitter) Update(object client.Object) bool {
	return e.Handler.Update(object)
}

func (e *genericObjectEmitter) Delete(object client.Object) bool {
	return e.Handler.Delete(object)
}

func ObjectEmitterWrapper(eventType enum.EventType,
	shouldUpdate func(client.Object) bool,
	tweakFunc func(client.Object),
) ObjectEmitter {
	eventData := genericpayload.GenericObjectBundle{}
	return NewGenericObjectEmitter(
		eventType,
		&eventData,
		NewGenericObjectHandler(&eventData),
		WithShouldUpdate(shouldUpdate),
		WithTweakFunc(tweakFunc),
	)
}
