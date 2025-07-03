package generic

import (
	"encoding/json"
	"fmt"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// MaxBundleBytes defines the maximum in-memory size (in bytes) of the JSON-encoded GenericBundle[T].
// This value closely approximates the payload size when written to disk or transmitted over the network.
// ⚠️ IMPORTANT: This limit is empirically validated. Setting it above 800 KB (e.g., 980 KB) may exceed
// default Kafka broker or client message size limits, potentially resulting in errors such as:
// "produce message: Broker: Message size too large"
const MaxBundleBytes = 800 * 1024 // 800 KiB

var log = logger.DefaultZapLogger()

type ObjectMetadata struct {
	ID        string `json:"id,omitempty"`
	Namespace string `json:"ns,omitempty"`
	Name      string `json:"name,omitempty"`
}

type GenericBundle[T any] struct {
	Create         []T              `json:"create,omitempty"`
	Update         []T              `json:"update,omitempty"`
	Delete         []ObjectMetadata `json:"delete,omitempty"`
	Resync         []T              `json:"resync,omitempty"`
	ResyncMetadata []ObjectMetadata `json:"resync_metadata,omitempty"`
}

func NewGenericBundle[T any]() *GenericBundle[T] {
	return &GenericBundle[T]{}
}

func (b *GenericBundle[T]) IsEmpty() bool {
	return len(b.Create) == 0 &&
		len(b.Update) == 0 &&
		len(b.Delete) == 0 &&
		len(b.Resync) == 0 &&
		len(b.ResyncMetadata) == 0
}

// Size returns the in-memory size in bytes of the JSON-encoded GenericBundle[T],
// not the actual disk size, but closely related if you're writing the JSON directly to disk.
func (b *GenericBundle[T]) Size() (int, error) {
	data, err := json.Marshal(b)
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func (b *GenericBundle[T]) Clean() {
	b.Create = nil
	b.Update = nil
	b.Delete = nil
	b.Resync = nil
	b.ResyncMetadata = nil
}

func (b *GenericBundle[T]) AddCreate(obj T) (bool, error) {
	return b.tryAdd(&b.Create, obj)
}

func (b *GenericBundle[T]) AddUpdate(obj T) (bool, error) {
	return b.tryAdd(&b.Update, obj)
}

func (b *GenericBundle[T]) AddResync(obj T) (bool, error) {
	return b.tryAdd(&b.Resync, obj)
}

func (b *GenericBundle[T]) AddDelete(meta ObjectMetadata) (bool, error) {
	b.Delete = append(b.Delete, meta)

	size, err := b.Size()
	if err != nil {
		b.Delete = b.Delete[:len(b.Delete)-1]
		return false, err
	}

	if size > MaxBundleBytes {
		if len(b.Delete) == 1 {
			log.Warnf("Metadata [%s/%s] exceeds bundle size: %d bytes", meta.Namespace, meta.Name, size)
			return true, nil
		}
		b.Delete = b.Delete[:len(b.Delete)-1]
		return false, nil
	}

	return true, nil
}

func (b *GenericBundle[T]) AddResyncMetadata(metas []ObjectMetadata) error {
	b.ResyncMetadata = append(b.ResyncMetadata, metas...)

	size, err := b.Size()
	if err != nil {
		return err
	}
	if size > MaxBundleBytes {
		return fmt.Errorf("resync metadata exceeds bundle size limit: %d bytes", size)
	}
	return nil
}

// tryAdd tries to append an object and checks for size constraint.
func (b *GenericBundle[T]) tryAdd(target *[]T, obj T) (bool, error) {
	*target = append(*target, obj)

	size, err := b.Size()
	if err != nil {
		*target = (*target)[:len(*target)-1]
		return false, err
	}

	if size > MaxBundleBytes {
		if len(*target) == 1 {
			log.Warnf("Object exceeds bundle size limit: %d bytes", size)
			return true, nil
		}
		*target = (*target)[:len(*target)-1]
		return false, nil
	}

	return true, nil
}
