package generic

import (
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ ObjectEmitter = &genericObjectEmitter{}

type genericObjectEmitter struct {
	*genericEmitter
	Handler
}

func NewGenericObjectEmitter(eventType enum.EventType, eventData interface{},
	handler Handler, opts ...EmitterOption) ObjectEmitter {
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

// func (h *genericEventEmitter) Delete(obj client.Object) {
// 	if h.payload.Delete(obj) {
// 		h.currentVersion.Incr()
// 	}
// }

// func (h *genericEventEmitter) Update(obj client.Object) {
// 	if h.tweakFunc != nil {
// 		h.tweakFunc(obj)
// 	}
// 	if h.payload.Update(obj) {
// 		h.currentVersion.Incr()
// 	}
// }

// func (h *genericEventEmitter) Delete(obj client.Object) {
// 	if h.payload.Delete(obj) {
// 		h.currentVersion.Incr()
// 	}
// }
