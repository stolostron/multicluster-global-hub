package helper

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelsField presents a "f:labels" field subfield of metadataField.
type LabelsField struct {
	Labels map[string]struct{} `json:"f:labels"`
}

// MetadataField presents a "f:metadata" field subfield of v1.FieldsV1.
type MetadataField struct {
	LabelsField `json:"f:metadata"`
}

// HasAnnotation returns a bool if the given annotation exists in annotations.
func HasAnnotation(obj metav1.Object, annotation string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	_, found := obj.GetAnnotations()[annotation]
	return found
}

// HasLabel returns a bool if the given label exists in labels.
func HasLabel(obj metav1.Object, label string) bool {
	if obj == nil || obj.GetLabels() == nil {
		return false
	}

	_, found := obj.GetLabels()[label]

	return found
}

// AddAnnotations adds the given annotations to the given object. if obj is nil or annotations are nil, it's a no-op.
func AddAnnotations(obj metav1.Object, annotations map[string]string) {
	if obj == nil || annotations == nil {
		return
	}
	if obj.GetAnnotations() == nil {
		obj.SetAnnotations(annotations)
		return
	}
	// if we got here, annotations on the obj are not nil, and given annotations are not nil.
	mergedAnnotations := obj.GetAnnotations()

	for key, value := range annotations {
		mergedAnnotations[key] = value
	}

	obj.SetAnnotations(mergedAnnotations)
}
