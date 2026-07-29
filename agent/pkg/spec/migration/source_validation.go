// Copyright (c) 2025 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package migration

import (
	"context"
	"fmt"
	"sync"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	cetypes "github.com/cloudevents/sdk-go/v2/types"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type localMigrationRecord struct {
	fromHub string
	toHub   string
	phase   string
}

var (
	localMigrationMu sync.RWMutex
	localMigrations  = make(map[string]localMigrationRecord)
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
	if source == "" || targetHub == "" {
		return false
	}
	if localMigrationSourceAllowed(source, targetHub) {
		return true
	}
	if c == nil {
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

func localMigrationSourceAllowed(source, targetHub string) bool {
	localMigrationMu.RLock()
	defer localMigrationMu.RUnlock()

	for _, migration := range localMigrations {
		if migration.fromHub != source || migration.toHub != targetHub {
			continue
		}
		switch migration.phase {
		case migrationv1alpha1.PhaseInitializing, migrationv1alpha1.PhaseDeploying:
			return true
		}
	}
	return false
}

// EnsureLocalMigrationCR records an in-flight migration on the target hub so deploying
// bundles from the registered source hub pass MigrationSourceAllowed validation.
// Managed hubs do not install the ManagedClusterMigration CRD, so state is kept in-memory.
func EnsureLocalMigrationCR(
	_ context.Context,
	_ client.Client,
	targetHub string,
	event *migrationbundle.MigrationTargetBundle,
	phase string,
) error {
	if event == nil || targetHub == "" {
		return fmt.Errorf("invalid migration CR inputs")
	}
	if event.FromHub == "" || len(event.ManagedClusters) == 0 {
		return nil
	}
	if event.ManagedServiceAccountName == "" {
		return fmt.Errorf("managedServiceAccountName is required to record local migration state")
	}

	localMigrationMu.Lock()
	defer localMigrationMu.Unlock()
	localMigrations[event.ManagedServiceAccountName] = localMigrationRecord{
		fromHub: event.FromHub,
		toHub:   targetHub,
		phase:   phase,
	}
	return nil
}

// DeleteLocalMigrationCR removes recorded local migration state after cleaning completes.
func DeleteLocalMigrationCR(_ context.Context, _ client.Client, migrationName string) error {
	if migrationName == "" {
		return nil
	}

	localMigrationMu.Lock()
	defer localMigrationMu.Unlock()
	delete(localMigrations, migrationName)
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
