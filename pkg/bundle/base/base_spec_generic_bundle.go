package base

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Manger to Agent: SpecGenericBundle bundle received from transport containing Objects/DeletedObjects.
type SpecGenericBundle struct {
	Objects        []*unstructured.Unstructured `json:"objects"`
	DeletedObjects []*unstructured.Unstructured `json:"deletedObjects"`
}

// NewGenericBundle returns a new instance of GenericBundle.
func NewSpecGenericBundle() *SpecGenericBundle {
	return &SpecGenericBundle{}
}
