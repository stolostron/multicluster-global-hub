package spec

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Manger to Agent: GenericSpecBundle bundle received from transport containing Objects/DeletedObjects.
type GenericSpecBundle struct {
	Objects        []*unstructured.Unstructured `json:"objects"`
	DeletedObjects []*unstructured.Unstructured `json:"deletedObjects"`
}

// NewGenericBundle returns a new instance of GenericBundle.
func NewGenericSpecBundle() *GenericSpecBundle {
	return &GenericSpecBundle{}
}
