// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clustermigration

import (
	"context"
	"fmt"
	"time"

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
	log.Debugf("handle migration status event:\n %s", evt)

	if expireStr, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyExpireTime]); err == nil {
		if expireTime, err := time.Parse(time.RFC3339, expireStr); err == nil {
			if time.Now().After(expireTime) {
				log.Infof("migration status event has expired (expirytime=%s), skip processing", expireStr)
				return nil
			}
		}
	}

	bundle := &migrationbundle.MigrationStatusBundle{}
	if err := evt.DataAs(bundle); err != nil {
		log.Error("failed to parse migrationBundle", "error", err)
		return err
	}

	subject := evt.Subject()
	if subject == "" {
		clusterName, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
		if err != nil || clusterName == "" {
			log.Error("failed to parse migrationBundle subject")
			return fmt.Errorf("migration event missing subject")
		}
		subject = clusterName
	}

	if subject != constants.CloudEventGlobalHubClusterName {
		return fmt.Errorf("expected to get the subject %s, but got %s",
			constants.CloudEventGlobalHubClusterName, subject)
	}

	hubClusterName := evt.Source()
	if hubClusterName == "" {
		return fmt.Errorf("failed to parse migrationBundle event source")
	}

	if bundle.Resync {
		log.Infof("status: resync migration, hub: %s", hubClusterName)
		migration.ResetMigrationStatus(hubClusterName)
		return nil
	}

	migrationId, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationId])
	if err != nil || migrationId == "" {
		if bundle.MigrationId != "" {
			migrationId = bundle.MigrationId
		} else {
			return fmt.Errorf("the hub %s should set the migrationId extension", hubClusterName)
		}
	}

	stage, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyMigrationStage])
	if err != nil || stage == "" {
		if bundle.Stage != "" {
			stage = bundle.Stage
		} else {
			return fmt.Errorf("the hub %s should set the migrationstage extension", hubClusterName)
		}
	}

	log.Infof("status: migration event, id: %s, hub: %s, stage: %s",
		migrationId, hubClusterName, stage)

	if stage == migrationv1alpha1.PhaseValidating && len(bundle.ManagedClusters) > 0 {
		migration.SetClusterList(migrationId, bundle.ManagedClusters)
		log.Infof("status: set cluster list, id: %s, clusters: %v", migrationId, bundle.ManagedClusters)
	}

	if bundle.FailedClustersReported {
		migration.SetFailedClusters(migrationId, hubClusterName, stage, bundle.FailedClusters)
		log.Infof("status: received failed clusters report, id: %s, hub: %s, clusters: %v",
			migrationId, hubClusterName, bundle.FailedClusters)
		return nil
	}

	if bundle.ErrMessage != "" {
		migration.SetErrorMessage(migrationId, hubClusterName, stage, bundle.ErrMessage)
		migration.SetClusterErrorMessage(migrationId, hubClusterName, stage, bundle.ClusterErrors)
		log.Infof("status: migration failed, id: %s, hub: %s, stage: %s, error: %s",
			migrationId, hubClusterName, stage, bundle.ErrMessage)
		if len(bundle.ClusterErrors) > 0 {
			log.Infof("status: cluster errors, id: %s, errors: %v", migrationId, bundle.ClusterErrors)
		}
	} else {
		migration.SetFinished(migrationId, hubClusterName, stage)
		log.Infof("status: migration stage completed, id: %s, hub: %s, stage: %s",
			migrationId, hubClusterName, stage)
	}

	return nil
}
