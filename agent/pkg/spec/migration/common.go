// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	// PauseAnnotation is the annotation key to pause Hive reconciliation
	PauseAnnotation = "hive.openshift.io/reconcile-pause"
)

var ztpLog = logger.DefaultZapLogger()

// PauseResources defines resources that need pause annotation during migration
var PauseResources = []schema.GroupVersionKind{
	{
		Group:   "hive.openshift.io",
		Version: "v1",
		Kind:    "ClusterDeployment",
	},
	{
		Group:   "extensions.hive.openshift.io",
		Version: "v1alpha1",
		Kind:    "ImageClusterInstall",
	},
}

// AddPauseAnnotations adds hive.openshift.io/reconcile-pause=true annotation to resources defined in PauseResources
func AddPauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range PauseResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
				return fmt.Errorf("failed to get %s: %w", gvk.Kind, err)
			}
			ztpLog.Debugf("%s not found for cluster %s, skipping pause annotation", gvk.Kind, clusterName)
			continue
		}

		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, types.NamespacedName{
				Name:      clusterName,
				Namespace: clusterName,
			}, resource); err != nil {
				return err
			}
			annotations := resource.GetAnnotations()
			if annotations == nil {
				annotations = make(map[string]string)
			}
			annotations[PauseAnnotation] = "true"
			resource.SetAnnotations(annotations)
			return c.Update(ctx, resource)
		}); err != nil {
			return fmt.Errorf("failed to add pause annotation to %s: %w", gvk.Kind, err)
		}
		ztpLog.Infof("added pause annotation to %s %s", gvk.Kind, clusterName)
	}

	return nil
}

// RemovePauseAnnotations removes hive.openshift.io/reconcile-pause annotation from resources defined in PauseResources
func RemovePauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range PauseResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				ztpLog.Debugf("%s not found for cluster %s, skipping pause annotation removal", gvk.Kind, clusterName)
				continue
			}
			return fmt.Errorf("failed to get %s: %w", gvk.Kind, err)
		}

		annotations := resource.GetAnnotations()
		if annotations == nil || annotations[PauseAnnotation] == "" {
			ztpLog.Debugf("%s %s has no pause annotation", gvk.Kind, clusterName)
			continue
		}

		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			if err := c.Get(ctx, types.NamespacedName{
				Name:      clusterName,
				Namespace: clusterName,
			}, resource); err != nil {
				return err
			}
			annotations := resource.GetAnnotations()
			if annotations != nil {
				delete(annotations, PauseAnnotation)
				resource.SetAnnotations(annotations)
			}
			return c.Update(ctx, resource)
		}); err != nil {
			return fmt.Errorf("failed to remove pause annotation from %s: %w", gvk.Kind, err)
		}
		ztpLog.Infof("removed pause annotation from %s %s", gvk.Kind, clusterName)
	}

	return nil
}

// RemovePauseResourcesFinalizers removes finalizers from resources defined in PauseResources
func RemovePauseResourcesFinalizers(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range PauseResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)
		resource.SetName(clusterName)
		resource.SetNamespace(clusterName)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				ztpLog.Debugf("%s not found for cluster %s, skipping finalizer removal", gvk.Kind, clusterName)
				continue
			}
			return fmt.Errorf("failed to get %s: %w", gvk.Kind, err)
		}

		if err := removeFinalizers(ctx, c, resource); err != nil {
			return fmt.Errorf("failed to remove finalizers from %s: %w", gvk.Kind, err)
		}
	}

	return nil
}

// removeFinalizers removes all finalizers from a resource
func removeFinalizers(ctx context.Context, c client.Client, obj client.Object) error {
	if obj == nil {
		return nil
	}

	if len(obj.GetFinalizers()) == 0 {
		log.Debugf("resource %s/%s has no finalizers", obj.GetNamespace(), obj.GetName())
		return nil
	}

	log.Infof("removing finalizers from resource %s/%s", obj.GetNamespace(), obj.GetName())
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get the latest version to avoid conflicts
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if apierrors.IsNotFound(err) {
				log.Debugf("resource %s/%s not found, skipping finalizer removal", obj.GetNamespace(), obj.GetName())
				return nil
			}
			return fmt.Errorf("failed to get resource: %w", err)
		}

		if len(obj.GetFinalizers()) == 0 {
			return nil
		}

		obj.SetFinalizers([]string{})
		return c.Update(ctx, obj)
	})
}
