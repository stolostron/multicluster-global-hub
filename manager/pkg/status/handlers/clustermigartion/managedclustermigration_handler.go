// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clustermigration

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/migration"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

type managedClusterMigrationHandler struct {
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	client        client.Client
}

func RegisterManagedClusterMigrationHandler(mgr ctrl.Manager, conflationManager *conflator.ConflationManager) {
	k := &managedClusterMigrationHandler{
		eventType:     string(enum.ManagedClusterMigrationType),
		eventSyncMode: enum.DeltaStateMode, // the migration event is not full bundle, it's an delta event handle one by one
		eventPriority: conflator.ManagedClusterMigrationPriority,
		client:        mgr.GetClient(),
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		k.eventPriority,
		k.eventSyncMode,
		k.eventType,
		k.handle,
	))
}

func (k *managedClusterMigrationHandler) handle(ctx context.Context, evt *cloudevents.Event) error {
	log.Debugw("handle migrationBundle", "cloudevents", evt)

	bundle := &migrationbundle.ManagedClusterMigrationBundle{}
	if err := evt.DataAs(bundle); err != nil {
		log.Error("failed to parse migrationBundle", "error", err)
		return err
	}

	clusterName, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	if err != nil {
		log.Error("failed to parse migrationBundle clusterName", "error", err)
		return err
	}

	if clusterName != constants.CloudEventGlobalHubClusterName {
		return fmt.Errorf("expected to get the the clusterName %s, but got %s",
			constants.CloudEventGlobalHubClusterName, clusterName)
	}

	hubClusterName := evt.Source()
	if hubClusterName == "" {
		return fmt.Errorf("failed to parse migrationBundle event source")
	}

	if bundle.Stage == migrationv1alpha1.ConditionTypeInitialized {
		migration.SetFinished(bundle.MigrationId, hubClusterName, migrationv1alpha1.PhaseInitializing)
	}

	if bundle.Stage == migrationv1alpha1.ConditionTypeDeployed {
		migration.SetFinished(bundle.MigrationId, hubClusterName, migrationv1alpha1.PhaseDeploying)
	}

	if bundle.Stage == migrationv1alpha1.ConditionTypeRegistered {
		if bundle.MigrationId == "" {
			return fmt.Errorf("the hub %s should set the migrationId", hubClusterName)
		}
		migration.SetFinished(bundle.MigrationId, hubClusterName, migrationv1alpha1.PhaseRegistering)
		migration.SetErrorMessage(bundle.MigrationId, hubClusterName,
			migrationv1alpha1.PhaseRegistering, bundle.ErrMessage)
	}

	if bundle.Stage == migrationv1alpha1.ConditionTypeCleaned {
		migration.SetFinished(bundle.MigrationId, hubClusterName, migrationv1alpha1.PhaseCleaning)
	}
	return nil
}
