package generic

import (
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
)

var _ bundle.Payload = (*GenericPayload)(nil)

type GenericPayload []client.Object

func (p *GenericPayload) Update(obj client.Object) bool {

	index := getObjectIndexByUID(obj.GetUID(), *p)
	if index == -1 { // object not found, need to add it to the bundle
		*p = append(*p, obj)
		return true
	}

	// if we reached here, object already exists in the bundle. check if we need to update the object
	if obj.GetResourceVersion() == (*p)[index].GetResourceVersion() {
		return false // update in bundle only if object changed. check for changes using resourceVersion field
	}

	(*p)[index] = obj
	return true
}

func (p *GenericPayload) Delete(obj client.Object) bool {

	index := getObjectIndexByObj(obj, *p)
	if index == -1 { // trying to delete object which doesn't exist
		return false
	}

	*p = append((*p)[:index], (*p)[index+1:]...) // remove from objects
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
