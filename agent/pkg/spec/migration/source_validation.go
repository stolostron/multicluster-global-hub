// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var migrationDeployAllowedGVKs = map[schema.GroupVersionKind]struct{}{
	{Group: "cluster.open-cluster-management.io", Version: "v1", Kind: "ManagedCluster"}:      {},
	{Group: "agent.open-cluster-management.io", Version: "v1", Kind: "KlusterletAddonConfig"}: {},
	{Group: "", Version: "v1", Kind: "Secret"}:                                                {},
	{Group: "", Version: "v1", Kind: "ConfigMap"}:                                             {},
	{Group: "", Version: "v1", Kind: "Namespace"}:                                             {},
	{Group: "work.open-cluster-management.io", Version: "v1", Kind: "ManifestWork"}:           {},
}

// IsMigrationDeployingEvent reports whether evt is a migration resource deploying event.
func IsMigrationDeployingEvent(evt *cloudevents.Event) bool {
	if evt == nil || evt.Type() != string(enum.ManagedClusterMigrationType) {
		return false
	}

	stage, err := cetypes.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationStage])
	return err == nil && stage == migrationv1alpha1.PhaseDeploying
}

// MigrationSourceAllowed returns true when source may publish migration events to targetHub.
// Manager orchestration uses global-hub; source-hub deploying uses the registered from hub.
func MigrationSourceAllowed(ctx context.Context, c client.Client, source, targetHub string) bool {
	if source == constants.CloudEventGlobalHubClusterName {
		return true
	}
	if source == "" || targetHub == "" || c == nil {
		return false
	}

	list := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := c.List(ctx, list); err != nil {
		return false
	}

	for i := range list.Items {
		migration := &list.Items[i]
		if migration.Spec.From != source || migration.Spec.To != targetHub {
			continue
		}
		switch migration.Status.Phase {
		case migrationv1alpha1.PhaseInitializing, migrationv1alpha1.PhaseDeploying:
			return true
		}
	}

	return false
}

// EnsureLocalMigrationCR records an in-flight migration on the target hub so deploying
// bundles from the registered source hub pass MigrationSourceAllowed validation.
func EnsureLocalMigrationCR(
	ctx context.Context,
	c client.Client,
	targetHub string,
	event *migrationbundle.MigrationTargetBundle,
	phase string,
) error {
	if c == nil || event == nil || targetHub == "" {
		return fmt.Errorf("invalid migration CR inputs")
	}
	if event.FromHub == "" || len(event.ManagedClusters) == 0 {
		return nil
	}
	if event.ManagedServiceAccountName == "" {
		return fmt.Errorf("managedServiceAccountName is required to record local migration state")
	}

	if err := ensureGHNamespace(ctx, c); err != nil {
		return fmt.Errorf("failed to ensure migration namespace: %w", err)
	}

	migrationCR := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      event.ManagedServiceAccountName,
			Namespace: constants.GHDefaultNamespace,
		},
		Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
			From:                    event.FromHub,
			To:                      targetHub,
			IncludedManagedClusters: event.ManagedClusters,
		},
	}
	if err := c.Create(ctx, migrationCR); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create local migration CR: %w", err)
		}
		existing := &migrationv1alpha1.ManagedClusterMigration{}
		if getErr := c.Get(ctx, types.NamespacedName{
			Name:      event.ManagedServiceAccountName,
			Namespace: constants.GHDefaultNamespace,
		}, existing); getErr != nil {
			return fmt.Errorf("failed to get existing local migration CR: %w", getErr)
		}
		migrationCR = existing
	}

	migrationCR.Spec.From = event.FromHub
	migrationCR.Spec.To = targetHub
	migrationCR.Spec.IncludedManagedClusters = event.ManagedClusters
	if err := c.Update(ctx, migrationCR); err != nil {
		return fmt.Errorf("failed to update local migration CR: %w", err)
	}

	migrationCR.Status.Phase = phase
	if err := c.Status().Update(ctx, migrationCR); err != nil {
		return fmt.Errorf("failed to update local migration CR status: %w", err)
	}
	return nil
}

func ensureGHNamespace(ctx context.Context, c client.Client) error {
	name := constants.GHDefaultNamespace
	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: name}, ns)
	switch {
	case apierrors.IsNotFound(err):
		return c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
	case err != nil:
		return err
	case ns.Status.Phase != corev1.NamespaceTerminating:
		return nil
	}

	waitErr := wait.PollUntilContextTimeout(ctx, 200*time.Millisecond, 30*time.Second, true,
		func(pollCtx context.Context) (bool, error) {
			current := &corev1.Namespace{}
			getErr := c.Get(pollCtx, types.NamespacedName{Name: name}, current)
			if apierrors.IsNotFound(getErr) {
				return true, nil
			}
			if getErr != nil {
				return false, getErr
			}
			return current.Status.Phase != corev1.NamespaceTerminating, nil
		})
	if waitErr != nil {
		return fmt.Errorf("waiting for namespace %q termination: %w", name, waitErr)
	}

	return c.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}})
}

// DeleteLocalMigrationCR removes the shadow migration CR after cleaning completes.
func DeleteLocalMigrationCR(ctx context.Context, c client.Client, migrationName string) error {
	if c == nil || migrationName == "" {
		return nil
	}
	migrationCR := &migrationv1alpha1.ManagedClusterMigration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migrationName,
			Namespace: constants.GHDefaultNamespace,
		},
	}
	if err := c.Delete(ctx, migrationCR); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete local migration CR: %w", err)
	}
	return nil
}

// IsMigrationDeployResourceAllowed validates GVK and cluster binding for deploying-stage resources.
func IsMigrationDeployResourceAllowed(resource *unstructured.Unstructured, clusterName string) bool {
	if resource == nil || clusterName == "" {
		return false
	}

	gvk := resource.GroupVersionKind()
	if gvk.Group == "rbac.authorization.k8s.io" {
		return false
	}

	if _, ok := migrationDeployAllowedGVKs[gvk]; !ok {
		return false
	}

	switch gvk.Kind {
	case "ManagedCluster":
		return resource.GetName() == clusterName
	case "KlusterletAddonConfig":
		return resource.GetName() == clusterName && resource.GetNamespace() == clusterName
	case "Namespace":
		return resource.GetName() == clusterName
	case "ManifestWork":
		return resource.GetNamespace() == clusterName
	case "Secret", "ConfigMap":
		return resource.GetNamespace() == clusterName
	default:
		return false
	}
}
