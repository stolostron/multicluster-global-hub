package helper

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	controllerName      = "leaf-hub-agent-sync"
	notFoundErrorSuffix = "not found"
)

// UpdateObject function updates a given k8s object.
func UpdateObject(ctx context.Context, k8sClient client.Client, obj *unstructured.Unstructured) error {
	objectBytes, err := obj.MarshalJSON()
	if err != nil {
		return fmt.Errorf("failed to update object - %w", err)
	}

	forceChanges := true

	if err := k8sClient.Patch(ctx, obj, client.RawPatch(types.ApplyPatchType, objectBytes), &client.PatchOptions{
		FieldManager: controllerName,
		Force:        &forceChanges,
	}); err != nil {
		return fmt.Errorf("failed to update object - %w", err)
	}

	return nil
}

// DeleteObject tries to delete the given object from k8s. returns error and true/false if object was deleted or not.
func DeleteObject(ctx context.Context, k8sClient client.Client, obj *unstructured.Unstructured) (bool, error) {
	if err := k8sClient.Delete(ctx, obj); err != nil {
		if !strings.HasSuffix(err.Error(), notFoundErrorSuffix) {
			return false, fmt.Errorf("failed to delete object - %w", err)
		}
		return false, nil
	}
	return true, nil
}
