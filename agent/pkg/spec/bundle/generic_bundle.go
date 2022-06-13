package bundle

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewGenericBundle returns a new instance of GenericBundle.
func NewGenericBundle() *GenericBundle {
	return &GenericBundle{}
}

// GenericBundle bundle received from transport containing Objects/DeletedObjects.
type GenericBundle struct {
	Objects        []*unstructured.Unstructured `json:"objects"`
	DeletedObjects []*unstructured.Unstructured `json:"deletedObjects"`
}
