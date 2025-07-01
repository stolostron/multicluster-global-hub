package generic

import (
	"encoding/json"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const MaxBundleBytes = 980 * 1024 // 980 KiB

var log = logger.DefaultZapLogger()

type GenericBundle struct {
	// delta mode for the resource stream/event
	Create []unstructured.Unstructured `json:"create"`
	Update []unstructured.Unstructured `json:"update"`
	Delete []unstructured.Unstructured `json:"delete"`

	// full mode for the resource snapshot
	Resync         []unstructured.Unstructured `json:"resync"`
	ResyncMetadata []unstructured.Unstructured `json:"resync_metadata"`
}

func NewGenericBundle() *GenericBundle {
	return &GenericBundle{}
}

func (b *GenericBundle) IsEmpty() bool {
	return len(b.Create) == 0 &&
		len(b.Update) == 0 &&
		len(b.Delete) == 0 &&
		len(b.Resync) == 0 &&
		len(b.ResyncMetadata) == 0
}

func (b *GenericBundle) Size() (int, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (b *GenericBundle) Clean() {
	b.Create = nil
	b.Update = nil
	b.Delete = nil
	b.Resync = nil
	b.ResyncMetadata = nil
}

// These public methods return wether the object was added successfully,
// returning false if the addition would exceed the bundle size limit.
func (b *GenericBundle) AddCreate(obj client.Object) (bool, error) {
	return b.tryAdd(&b.Create, obj, false)
}

func (b *GenericBundle) AddUpdate(obj client.Object) (bool, error) {
	return b.tryAdd(&b.Update, obj, false)
}

func (b *GenericBundle) AddResync(obj client.Object) (bool, error) {
	return b.tryAdd(&b.Resync, obj, false)
}

func (b *GenericBundle) AddDelete(obj client.Object) (bool, error) {
	return b.tryAdd(&b.Delete, obj, true)
}

func (b *GenericBundle) AddResyncMetadata(objects []client.Object) error {
	for _, obj := range objects {
		u := &unstructured.Unstructured{}
		u.SetNamespace(obj.GetNamespace())
		u.SetName(obj.GetName())
		u.SetUID(obj.GetUID())
		// Optional: u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		b.ResyncMetadata = append(b.ResyncMetadata, *u)
	}
	size, err := b.Size()
	if err != nil {
		return err
	}
	if size > MaxBundleBytes {
		log.Warnf("Metadatas [%s] exceeds the max bundle size limit: %d bytes", len(objects), size)
	}
	return nil
}

// for deleting or resyncing metadata, only update the metadata into the bundle
func (b *GenericBundle) tryAdd(target *[]unstructured.Unstructured, obj client.Object, metadataOnly bool) (
	bool, error,
) {
	var u *unstructured.Unstructured
	var err error

	if metadataOnly {
		u := &unstructured.Unstructured{}
		u.SetNamespace(obj.GetNamespace())
		u.SetName(obj.GetName())
		u.SetUID(obj.GetUID())
		// u.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
	} else {
		tmpMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		if err != nil {
			return false, err
		}
		tmp := &unstructured.Unstructured{Object: tmpMap}
		tmp.SetGroupVersionKind(obj.GetObjectKind().GroupVersionKind())
		u = tmp
	}

	*target = append(*target, *u)

	size, err := b.Size()
	if err != nil {
		*target = (*target)[:len(*target)-1]
		return false, err
	}

	if size > MaxBundleBytes {
		if len(*target) == 1 {
			log.Warnf("Object [%s/%s] exceeds the max bundle size limit: %d bytes", obj.GetNamespace(), obj.GetName(), size)
			return true, nil
		}

		*target = (*target)[:len(*target)-1]
		return false, nil
	}

	return true, nil
}
