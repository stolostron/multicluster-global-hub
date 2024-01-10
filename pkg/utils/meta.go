package utils

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
func HasLabelKey(labels map[string]string, label string) bool {
	if len(labels) == 0 {
		return false
	}
	_, found := labels[label]
	return found
}

// HasLabel check if the labels has key=value label
func HasLabel(labels map[string]string, key, value string) bool {
	if len(labels) == 0 {
		return false
	}
	for k, v := range labels {
		if k == key && v == value {
			return true
		}
	}
	return false
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

// Remove is used to remove string from a string array
func Remove(list []string, s string) []string {
	result := []string{}
	for _, v := range list {
		if v != s {
			result = append(result, v)
		}
	}
	return result
}

// Contains is used to check whether a list contains string s
func Contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// GetAnnotation returns the annotation value for a given key, or an empty string if not set
func GetAnnotation(annotations map[string]string, key string) string {
	if annotations == nil {
		return ""
	}
	return annotations[key]
}

func RemoveDuplicates(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]struct{}{}
	result := []string{}

	for _, v := range elements {
		if _, found := encountered[v]; found {
			continue
		}
		encountered[v] = struct{}{}
		result = append(result, v)
	}
	// Return the new slice.
	return result
}

func UpdateObject(ctx context.Context, runtimeClient client.Client, obj client.Object) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		return runtimeClient.Update(ctx, obj, &client.UpdateOptions{})
	})
}
