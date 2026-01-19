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
	// HivePauseAnnotation is the annotation key to pause Hive reconciliation
	HivePauseAnnotation = "hive.openshift.io/reconcile-pause"

	// Metal3PauseAnnotation is the annotation key to pause Metal3 reconciliation
	Metal3PauseAnnotation = "baremetalhost.metal3.io/paused"

	// errFailedToGetResourceFmt is the error message format for resource retrieval failures
	errFailedToGetResourceFmt = "failed to get %s: %w"
)

var ztpLog = logger.DefaultZapLogger()

// ZTPResourceConfig contains the GVK, pause annotation and finalizer suffix for a ZTP resource type
type ZTPResourceConfig struct {
	GVK             schema.GroupVersionKind
	PauseAnnotation string
	FinalizerSuffix string // Finalizers with this suffix will be removed
}

// ZTPClusterResources defines the specific ZTP cluster resource types that need pause annotation during migration
var ZTPClusterResources = []ZTPResourceConfig{
	{
		GVK: schema.GroupVersionKind{
			Group:   "hive.openshift.io",
			Version: "v1",
			Kind:    "ClusterDeployment",
		},
		PauseAnnotation: HivePauseAnnotation,
		FinalizerSuffix: "/deprovision",
	},
	{
		GVK: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "BareMetalHost",
		},
		PauseAnnotation: Metal3PauseAnnotation,
		FinalizerSuffix: ".metal3.io",
	},
	{
		GVK: schema.GroupVersionKind{
			Group:   "metal3.io",
			Version: "v1alpha1",
			Kind:    "DataImage",
		},
		PauseAnnotation: Metal3PauseAnnotation,
		FinalizerSuffix: ".metal3.io",
	},
}

// AddPauseAnnotations adds pause annotations to resources defined in ZTPClusterResources
// Each resource type uses its specific pause annotation (e.g., hive.openshift.io/reconcile-pause
// for ClusterDeployment, baremetalhost.metal3.io/paused for BareMetalHost and DataImage)
// Iterates through all ZTP resource types to ensure each is properly paused during migration
func AddPauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, ztpResource := range ZTPClusterResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(ztpResource.GVK)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if !apierrors.IsNotFound(err) && !meta.IsNoMatchError(err) {
				return fmt.Errorf(errFailedToGetResourceFmt, ztpResource.GVK.Kind, err)
			}
			ztpLog.Debugf("%s not found for cluster %s, skipping pause annotation", ztpResource.GVK.Kind, clusterName)
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
			annotations[ztpResource.PauseAnnotation] = "true"
			resource.SetAnnotations(annotations)
			return c.Update(ctx, resource)
		}); err != nil {
			return fmt.Errorf("failed to add pause annotation to %s: %w", ztpResource.GVK.Kind, err)
		}
		ztpLog.Infof("added pause annotation %s to %s %s", ztpResource.PauseAnnotation, ztpResource.GVK.Kind, clusterName)
	}

	return nil
}

// RemovePauseAnnotations removes pause annotations from resources defined in ZTPClusterResources
// Each resource type uses its specific pause annotation (e.g., hive.openshift.io/reconcile-pause
// for ClusterDeployment, baremetalhost.metal3.io/paused for BareMetalHost and DataImage)
// Iterates through all ZTP resource types to ensure pause annotations are cleaned up in rollback
// stage when migration failed
func RemovePauseAnnotations(ctx context.Context, c client.Client, clusterName string) error {
	for _, ztpResource := range ZTPClusterResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(ztpResource.GVK)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				ztpLog.Debugf("%s not found for cluster %s, skipping pause annotation removal", ztpResource.GVK.Kind, clusterName)
				continue
			}
			return fmt.Errorf(errFailedToGetResourceFmt, ztpResource.GVK.Kind, err)
		}

		annotations := resource.GetAnnotations()
		if annotations == nil || annotations[ztpResource.PauseAnnotation] == "" {
			ztpLog.Debugf("%s %s has no pause annotation", ztpResource.GVK.Kind, clusterName)
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
				delete(annotations, ztpResource.PauseAnnotation)
				resource.SetAnnotations(annotations)
			}
			return c.Update(ctx, resource)
		}); err != nil {
			return fmt.Errorf("failed to remove pause annotation from %s: %w", ztpResource.GVK.Kind, err)
		}
		ztpLog.Infof("removed pause annotation %s from %s %s", ztpResource.PauseAnnotation, ztpResource.GVK.Kind, clusterName)
	}

	return nil
}

// RemoveDeprovisionFinalizers removes finalizers from resources defined in ZTPClusterResources
// Each resource type has its specific finalizer suffix pattern:
// - ClusterDeployment: finalizers ending with "/deprovision"
// - BareMetalHost and DataImage: finalizers ending with ".metal3.io"
func RemoveDeprovisionFinalizers(ctx context.Context, c client.Client, clusterName string) error {
	for _, ztpResource := range ZTPClusterResources {
		resource := &unstructured.Unstructured{}
		resource.SetGroupVersionKind(ztpResource.GVK)
		resource.SetName(clusterName)
		resource.SetNamespace(clusterName)

		if err := c.Get(ctx, types.NamespacedName{
			Name:      clusterName,
			Namespace: clusterName,
		}, resource); err != nil {
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				ztpLog.Debugf("%s not found for cluster %s, skipping finalizer removal", ztpResource.GVK.Kind, clusterName)
				continue
			}
			return fmt.Errorf(errFailedToGetResourceFmt, ztpResource.GVK.Kind, err)
		}

		if err := removeFinalizersWithSuffix(ctx, c, resource, ztpResource.FinalizerSuffix); err != nil {
			return fmt.Errorf("failed to remove finalizers with suffix %s from %s: %w",
				ztpResource.FinalizerSuffix, ztpResource.GVK.Kind, err)
		}
	}

	return nil
}

// removeFinalizersWithSuffix removes finalizers with the specified suffix from a resource
func removeFinalizersWithSuffix(ctx context.Context, c client.Client, obj client.Object, suffix string) error {
	return removeFinalizersWithFilter(ctx, c, obj, func(f string) bool {
		return strings.HasSuffix(f, suffix)
	}, suffix)
}

// removeDeprovisionFinalizers removes only finalizers with /deprovision suffix from a resource
func removeDeprovisionFinalizers(ctx context.Context, c client.Client, obj client.Object) error {
	return removeFinalizersWithSuffix(ctx, c, obj, "/deprovision")
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

// filterByLabelKey filters resources that have the specified label key
func filterByLabelKey(items []unstructured.Unstructured, labelKey string) []unstructured.Unstructured {
	filteredItems := make([]unstructured.Unstructured, 0)
	for _, item := range items {
		if hasLabelKey(&item, labelKey) {
			filteredItems = append(filteredItems, item)
		}
	}
	return filteredItems
}

// hasLabelKey checks if a resource has the specified label key
func hasLabelKey(item *unstructured.Unstructured, labelKey string) bool {
	labels := item.GetLabels()
	if labels == nil {
		return false
	}
	_, exists := labels[labelKey]
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

// listAndFilterResources lists all resources in a namespace and optionally filters by annotation key or label key
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

	// Filter by label key if specified
	if migrateResource.labelKey != "" {
		resourceList.Items = filterByLabelKey(resourceList.Items, migrateResource.labelKey)
		log.Debugf("filtered %d resources with label key %s in namespace %s",
			len(resourceList.Items), migrateResource.labelKey, clusterName)
	}

	// If no resources found, return empty array
	if len(resourceList.Items) == 0 {
		log.Warnf("no resources found: GVK=%s, Namespace=%s, AnnotationKey=%s, LabelKey=%s",
			migrateResource.gvk.String(), clusterName, migrateResource.annotationKey, migrateResource.labelKey)
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
