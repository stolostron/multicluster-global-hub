// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"fmt"
	"strings"

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

	// errFailedToGetResourceFmt is the error message format for resource retrieval failures
	errFailedToGetResourceFmt = "failed to get %s: %w"
)

var ztpLog = logger.DefaultZapLogger()

// ZTPClusterResourceGVKs defines the specific ZTP cluster resource types
// (ClusterDeployment and ImageClusterInstall) that need pause annotation during migration
var ZTPClusterResourceGVKs = []schema.GroupVersionKind{
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

// AddPauseAnnotations adds hive.openshift.io/reconcile-pause=true annotation
// to resources defined in ZTPClusterResourceGVKs
func AddPauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range ZTPClusterResourceGVKs {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(gvk)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
				return fmt.Errorf(errFailedToGetResourceFmt, gvk.Kind, err)
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

// RemovePauseAnnotations removes hive.openshift.io/reconcile-pause annotation
// from resources defined in ZTPClusterResourceGVKs
func RemovePauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range ZTPClusterResourceGVKs {
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
			return fmt.Errorf(errFailedToGetResourceFmt, gvk.Kind, err)
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

// RemoveDeprovisionFinalizers removes /deprovision finalizers from resources defined in ZTPClusterResourceGVKs
func RemoveDeprovisionFinalizers(ctx context.Context, c client.Client, clusterName string) error {
	for _, gvk := range ZTPClusterResourceGVKs {
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
			return fmt.Errorf(errFailedToGetResourceFmt, gvk.Kind, err)
		}

		if err := removeDeprovisionFinalizers(ctx, c, resource); err != nil {
			return fmt.Errorf("failed to remove deprovision finalizers from %s: %w", gvk.Kind, err)
		}
	}

	return nil
}

// removeDeprovisionFinalizers removes only finalizers with /deprovision suffix from a resource
func removeDeprovisionFinalizers(ctx context.Context, c client.Client, obj client.Object) error {
	return removeFinalizersWithFilter(ctx, c, obj, func(f string) bool {
		return strings.HasSuffix(f, "/deprovision")
	}, "/deprovision")
}

// removeFinalizers removes all finalizers from a resource
func removeFinalizers(ctx context.Context, c client.Client, obj client.Object) error {
	return removeFinalizersWithFilter(ctx, c, obj, func(f string) bool {
		return true
	}, "all")
}

// removeFinalizersWithFilter removes finalizers matching the filter predicate from a resource
// filterFn returns true for finalizers that should be removed
// filterDesc is used for logging to describe what finalizers are being removed
func removeFinalizersWithFilter(ctx context.Context, c client.Client, obj client.Object,
	filterFn func(string) bool, filterDesc string,
) error {
	if obj == nil {
		return nil
	}

	if len(obj.GetFinalizers()) == 0 {
		log.Debugf("resource %s/%s has no finalizers", obj.GetNamespace(), obj.GetName())
		return nil
	}

	log.Infof("removing %s finalizers from resource %s/%s", filterDesc, obj.GetNamespace(), obj.GetName())
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		// Get the latest version to avoid conflicts
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			if apierrors.IsNotFound(err) {
				log.Debugf("resource %s/%s not found, skipping finalizer removal", obj.GetNamespace(), obj.GetName())
				return nil
			}
			return fmt.Errorf("failed to get resource: %w", err)
		}

		finalizers := obj.GetFinalizers()
		if len(finalizers) == 0 {
			return nil
		}

		// Filter out finalizers matching the predicate
		filteredFinalizers := make([]string, 0, len(finalizers))
		for _, f := range finalizers {
			if !filterFn(f) {
				filteredFinalizers = append(filteredFinalizers, f)
			}
		}

		// Only update if we actually removed any finalizers
		if len(filteredFinalizers) == len(finalizers) {
			return nil
		}

		obj.SetFinalizers(filteredFinalizers)
		return c.Update(ctx, obj)
	})
}
