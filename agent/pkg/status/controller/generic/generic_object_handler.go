package generic

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
)

type genericObjectHandler struct {
	eventData genericpayload.GenericObjectData
}

func NewGenericObjectHandler(eventData genericpayload.GenericObjectData) Handler {
	return &genericObjectHandler{
		eventData: eventData,
	}
}

func (h *genericObjectHandler) Update(obj client.Object) bool {

	index := getObjectIndexByUID(obj.GetUID(), h.eventData)
	if index == -1 { // object not found, need to add it to the bundle
		h.eventData = append(h.eventData, obj)
		return true
	}

	// if we reached here, object already exists in the bundle. check if we need to update the object
	if obj.GetResourceVersion() == h.eventData[index].GetResourceVersion() {
		return false // update in bundle only if object changed. check for changes using resourceVersion field
	}

	h.eventData[index] = obj
	return true
}

func (h *genericObjectHandler) Delete(obj client.Object) bool {

	index := getObjectIndexByObj(obj, h.eventData)
	if index == -1 { // trying to delete object which doesn't exist
		return false
	}

	h.eventData = append(h.eventData[:index], h.eventData[index+1:]...) // remove from objects
	return true
}

func getObjectIndexByUID(uid types.UID, objects []client.Object) int {
	for i, object := range objects {
		if object.GetUID() == uid {
			return i
		}
	}
	return -1
}

func getObjectIndexByObj(obj client.Object, objects []client.Object) int {
	if len(obj.GetUID()) > 0 {
		return getObjectIndexByUID(obj.GetUID(), objects)
	} else {
		for i, object := range objects {
			if object.GetNamespace() == obj.GetNamespace() && object.GetName() == obj.GetName() {
				return i
			}
		}
	}
	return -1
}
