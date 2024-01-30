package utils

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog"
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

func AddLabel(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	labelKey string, labelValue string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, obj)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			klog.Errorf("Failed to get %v/%v, err:%v", namespace, name, err)
			return err
		}
		if HasLabel(obj.GetLabels(), labelKey, labelValue) {
			return nil
		}

		objNewLabels := obj.GetLabels()

		if objNewLabels == nil {
			objNewLabels = make(map[string]string)
		}
		objNewLabels[labelKey] = labelValue
		obj.SetLabels(objNewLabels)
		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update %v/%v, err:%v", namespace, name, err)
			return err
		}
		return nil
	})
}

func DeleteLabel(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	labelKey string,
) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		err := client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      name,
		}, obj)
		if err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			klog.Errorf("Failed to get %v/%v, err:%v", namespace, name, err)
			return err
		}

		if !HasLabelKey(obj.GetLabels(), labelKey) {
			return nil
		}

		objNewLabels := obj.GetLabels()
		delete(objNewLabels, labelKey)
		obj.SetLabels(objNewLabels)

		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update %v/%v, err:%v", namespace, name, err)
			return err
		}
		return nil
	})
}
