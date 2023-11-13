package generic

import (
	"sync"

	"k8s.io/apimachinery/pkg/types"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
)

// NewGenericStatusBundle creates a new instance of GenericStatusBundle.
func NewGenericStatusBundle(leafHubName string, manipulateObjFunc func(obj bundle.Object)) bundle.Bundle {
	if manipulateObjFunc == nil {
		manipulateObjFunc = func(object bundle.Object) {
			// do nothing
		}
	}

	return &GenericStatusBundle{
		Objects:           make([]bundle.Object, 0),
		LeafHubName:       leafHubName,
		BundleVersion:     status.NewBundleVersion(),
		manipulateObjFunc: manipulateObjFunc,
		lock:              sync.Mutex{},
	}
}

// GenericStatusBundle is a bundle that is used to send to the hub of hubs the leaf CR as is
// except for fields that are not relevant in the hub of hubs like finalizers, etc.
// for bundles that require more specific behavior, it's required to implement your own status bundle struct.
type GenericStatusBundle struct {
	Objects           []bundle.Object       `json:"objects"`
	LeafHubName       string                `json:"leafHubName"`
	BundleVersion     *status.BundleVersion `json:"bundleVersion"`
	manipulateObjFunc func(obj bundle.Object)
	lock              sync.Mutex
}

// UpdateObject function to update a single object inside a bundle.
func (genericBundle *GenericStatusBundle) UpdateObject(object bundle.Object) {
	genericBundle.lock.Lock()
	defer genericBundle.lock.Unlock()

	genericBundle.manipulateObjFunc(object)

	index, err := genericBundle.getObjectIndexByUID(object.GetUID())
	if err != nil { // object not found, need to add it to the bundle
		genericBundle.Objects = append(genericBundle.Objects, object)
		genericBundle.BundleVersion.Incr()
		return
	}

	// if we reached here, object already exists in the bundle. check if we need to update the object
	if object.GetResourceVersion() == genericBundle.Objects[index].GetResourceVersion() {
		return // update in bundle only if object changed. check for changes using resourceVersion field
	}

	genericBundle.Objects[index] = object
	genericBundle.BundleVersion.Incr()
}

// DeleteObject function to delete a single object inside a bundle.
func (generic *GenericStatusBundle) DeleteObject(object bundle.Object) {
	generic.lock.Lock()
	defer generic.lock.Unlock()

	index, err := generic.getObjectIndexByObj(object)
	if err != nil { // trying to delete object which doesn't exist - return with no error
		return
	}
	generic.Objects = append(generic.Objects[:index], generic.Objects[index+1:]...) // remove from objects
	generic.BundleVersion.Incr()
}

// GetBundleVersion function to get bundle version.
func (bundle *GenericStatusBundle) GetBundleVersion() *status.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (genericbundle *GenericStatusBundle) getObjectIndexByUID(uid types.UID) (int, error) {
	for i, object := range genericbundle.Objects {
		if object.GetUID() == uid {
			return i, nil
		}
	}

	return -1, bundle.ErrObjectNotFound
}

func (genericBundle *GenericStatusBundle) getObjectIndexByObj(obj bundle.Object) (int, error) {
	if len(obj.GetUID()) > 0 {
		for i, object := range genericBundle.Objects {
			if object.GetUID() == obj.GetUID() {
				return i, nil
			}
		}
	} else {
		for i, object := range genericBundle.Objects {
			if object.GetNamespace() == obj.GetNamespace() && object.GetName() == obj.GetName() {
				return i, nil
			}
		}
	}
	return -1, bundle.ErrObjectNotFound
}
