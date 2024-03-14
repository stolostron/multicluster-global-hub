package spec

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Manger to Agent: GenericSpecData bundle received from transport containing Objects/DeletedObjects.
type GenericSpecData struct {
	Objects        []*unstructured.Unstructured `json:"objects"`
	DeletedObjects []*unstructured.Unstructured `json:"deletedObjects"`
}

// NewGenericBundle returns a new instance of GenericBundle.
func NewGenericSpecData() *GenericSpecData {
	return &GenericSpecData{}
}
