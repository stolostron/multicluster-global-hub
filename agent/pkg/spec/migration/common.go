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

// filterByAnnotationKey filters resources that have the specified annotation key
func filterByAnnotationKey(items []unstructured.Unstructured, annotationKey string) []unstructured.Unstructured {
	filteredItems := make([]unstructured.Unstructured, 0)
	for _, item := range items {
		if hasAnnotationKey(&item, annotationKey) {
			filteredItems = append(filteredItems, item)
		}
	}
	return filteredItems
}

// hasAnnotationKey checks if a resource has the specified annotation key
func hasAnnotationKey(item *unstructured.Unstructured, annotationKey string) bool {
	annotations := item.GetAnnotations()
	if annotations == nil {
		return false
	}
	_, exists := annotations[annotationKey]
	return exists
}

// prepareUnstructuredResourceForMigration prepares an unstructured resource for migration by cleaning metadata
// The resource name and namespace should match the managed cluster name
func prepareUnstructuredResourceForMigration(
	ctx context.Context,
	c client.Client,
	clusterName string,
	migrateResource MigrationResource,
) ([]unstructured.Unstructured, error) {
	var resources []unstructured.Unstructured

	if migrateResource.name != "" {
		// Replace <CLUSTER_NAME> placeholder with actual cluster name
		resourceName := strings.ReplaceAll(migrateResource.name, "<CLUSTER_NAME>", clusterName)

		// Get specific resource by name
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(migrateResource.gvk)
		if err := c.Get(ctx, types.NamespacedName{
			Name:      resourceName,
			Namespace: clusterName,
		}, resource); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				return resources, nil
			}
			return nil, fmt.Errorf("failed to get %v %s: %w", migrateResource.gvk, clusterName, err)
		}
		resources = append(resources, *resource)
	} else {
		listedResources, err := listAndFilterResources(ctx, c, clusterName, migrateResource)
		if err != nil {
			return nil, err
		}
		resources = append(resources, listedResources...)
	}

	return resources, nil
}

// listAndFilterResources lists all resources in a namespace and optionally filters by annotation key
func listAndFilterResources(
	ctx context.Context,
	c client.Client,
	clusterName string,
	migrateResource MigrationResource,
) ([]unstructured.Unstructured, error) {
	// List all resources in the cluster namespace
	resourceList := &unstructured.UnstructuredList{}
	resourceList.SetGroupVersionKind(migrateResource.gvk)

	if err := c.List(ctx, resourceList, client.InNamespace(clusterName)); err != nil {
		if meta.IsNoMatchError(err) {
			log.Warnf("resource kind not registered: GVK=%s, Namespace=%s, error=%v",
				migrateResource.gvk.String(), clusterName, err)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list %v in namespace %s: %w", migrateResource.gvk, clusterName, err)
	}

	// Filter by annotation key if specified
	if migrateResource.annotationKey != "" {
		resourceList.Items = filterByAnnotationKey(resourceList.Items, migrateResource.annotationKey)
		log.Debugf("filtered %d resources with annotation key %s in namespace %s",
			len(resourceList.Items), migrateResource.annotationKey, clusterName)
	}

	// If no resources found, return empty array
	if len(resourceList.Items) == 0 {
		log.Warnf("no resources found: GVK=%s, Namespace=%s, AnnotationKey=%s",
			migrateResource.gvk.String(), clusterName, migrateResource.annotationKey)
		return nil, nil
	}

	log.Debugf("found %d resources of type %s in namespace %s",
		len(resourceList.Items), migrateResource.gvk.String(), clusterName)
	return resourceList.Items, nil
}

// collectMigrationResources collects and prepares all migration resources for a managed cluster
func collectMigrationResources(
	ctx context.Context,
	c client.Client,
	managedCluster string,
	migrateResources []MigrationResource,
	cleanObjectMetadataFn func(client.Object),
	processResourceByTypeFn func(*unstructured.Unstructured, MigrationResource),
) ([]unstructured.Unstructured, error) {
	var resourcesList []unstructured.Unstructured

	// collect all defined migration resources
	for _, migrateResource := range migrateResources {
		resources, err := prepareUnstructuredResourceForMigration(
			ctx, c, managedCluster, migrateResource)
		if err != nil {
			return nil, fmt.Errorf("failed to prepare %s %s for migration: %w", migrateResource.gvk.Kind, managedCluster, err)
		}

		for i := range resources {
			cleanResourceForMigration(&resources[i], migrateResource, cleanObjectMetadataFn, processResourceByTypeFn)
		}

		resourcesList = append(resourcesList, resources...)
	}

	return resourcesList, nil
}

// cleanResourceForMigration cleans up a resource for migration
func cleanResourceForMigration(
	resource *unstructured.Unstructured,
	migrateResource MigrationResource,
	cleanObjectMetadataFn func(client.Object),
	processResourceByTypeFn func(*unstructured.Unstructured, MigrationResource),
) {
	// Clean metadata for migration
	cleanObjectMetadataFn(resource)

	// Apply resource-specific processing based on resource type
	processResourceByTypeFn(resource, migrateResource)

	// Clean status field based on needStatus configuration
	if !migrateResource.needStatus {
		// Remove status field if not needed for migration
		unstructured.RemoveNestedField(resource.Object, "status")
	}

	// Remove any kubectl last-applied-configuration annotations
	annotations := resource.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		resource.SetAnnotations(annotations)
	}
}
