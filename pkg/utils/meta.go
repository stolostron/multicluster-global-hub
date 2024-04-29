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

const faildGetMsg = "Failed to get %v/%v, err:%v"

// LabelsField presents a "f:labels" field subfield of metadataField.
type LabelsField struct {
	Labels map[string]struct{} `json:"f:labels"`
}

// MetadataField presents a "f:metadata" field subfield of v1.FieldsV1.
type MetadataField struct {
	LabelsField `json:"f:metadata"`
}

// HasAnnotation returns a bool if the given annotation exists in annotations.
func HasAnnotation(obj metav1.Object, key string) bool {
	if obj == nil || obj.GetAnnotations() == nil {
		return false
	}
	_, found := obj.GetAnnotations()[key]
	return found
}

// HasAnnotation returns a bool if the given annotation exists in annotations.
func HasLabel(obj metav1.Object, key string) bool {
	if obj == nil || obj.GetLabels() == nil {
		return false
	}
	_, found := obj.GetLabels()[key]
	return found
}

// HasItem returns a bool if the given label exists in labels.
func HasItemKey(labels map[string]string, label string) bool {
	if len(labels) == 0 {
		return false
	}
	_, found := labels[label]
	return found
}

// HasItem check if the labels has key=value label
func HasItem(labels map[string]string, key, value string) bool {
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

// MergeAnnotations adds the given annotations to the given object. if obj is nil or annotations are nil, it's a no-op.
func MergeAnnotations(obj metav1.Object, annotations map[string]string) {
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

func AddAnnotation(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	key string, value string,
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
			klog.Errorf(faildGetMsg, namespace, name, err)
			return err
		}
		if HasItem(obj.GetAnnotations(), key, value) {
			return nil
		}

		objNewAnnotation := obj.GetAnnotations()

		if objNewAnnotation == nil {
			objNewAnnotation = make(map[string]string)
		}
		objNewAnnotation[key] = value
		obj.SetAnnotations(objNewAnnotation)
		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update %v/%v with err:%v", namespace, name, err)
			return err
		}
		return nil
	})
}

func DeleteAnnotation(
	ctx context.Context, client client.Client, obj client.Object,
	namespace, name string,
	key string,
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
			klog.Errorf(faildGetMsg, namespace, name, err)
			return err
		}
		if !HasItemKey(obj.GetAnnotations(), key) {
			return nil
		}

		objNewAnnotations := obj.GetAnnotations()
		delete(objNewAnnotations, key)
		obj.SetAnnotations(objNewAnnotations)
		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update obj %v/%v with err:%v", namespace, name, err)
			return err
		}
		return nil
	})
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
			klog.Errorf(faildGetMsg, namespace, name, err)
			return err
		}
		if HasItem(obj.GetLabels(), labelKey, labelValue) {
			return nil
		}

		objNewLabels := obj.GetLabels()

		if objNewLabels == nil {
			objNewLabels = make(map[string]string)
		}
		objNewLabels[labelKey] = labelValue
		obj.SetLabels(objNewLabels)
		if err := client.Update(ctx, obj); err != nil {
			klog.Errorf("Failed to update obj %v/%v, err:%v", namespace, name, err)
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
			klog.Errorf(faildGetMsg, namespace, name, err)
			return err
		}

		if !HasItemKey(obj.GetLabels(), labelKey) {
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
