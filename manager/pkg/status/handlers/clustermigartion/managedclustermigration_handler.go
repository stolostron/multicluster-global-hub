// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package clustermigration

import (
	"context"
	"encoding/json"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/migration"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
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
		eventSyncMode: enum.CompleteStateMode,
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

	eventClusterName, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	if err != nil {
		log.Error("failed to parse migrationBundle clusterName", "error", err)
		return err
	}

	db := database.GetGorm()

	// from source hub -> resource initialized
	if bundle.Stage == migrationv1alpha1.ConditionTypeInitialized {
		klusterletAddonConfigData, err := json.Marshal(bundle.KlusterletAddonConfig)
		if err != nil {
			return err
		}
		cluster := bundle.KlusterletAddonConfig.Name
		mcm := models.ManagedClusterMigration{
			FromHub:     evt.Source(),
			ToHub:       eventClusterName,
			ClusterName: cluster,
			Payload:     klusterletAddonConfigData,
			Stage:       migrationv1alpha1.ConditionTypeInitialized,
		}

		err = db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "from_hub"}, {Name: "to_hub"}, {Name: "cluster_name"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"payload": klusterletAddonConfigData,
			}),
		}).Create(&mcm).Error
		if err != nil {
			log.Errorf("failed to update the migration initializing data into db: %v", err)
			return err
		}
	}

	// from destination hub -> resource deployed
	if bundle.Stage == migrationv1alpha1.ConditionTypeDeployed {
		for _, cluster := range bundle.ManagedClusters {
			err = db.Model(&models.ManagedClusterMigration{}).
				Where("to_hub = ?", evt.Source()).
				Where("cluster_name = ?", cluster).
				Update("stage", migrationv1alpha1.ConditionTypeDeployed).Error
			if err != nil {
				log.Errorf("failed to mark the stage ResourceDeployed for %s - %s in db: %v", evt.Source(), cluster, err)
				return err
			}
		}
	}

	// from source hub -> migration completed
	if bundle.Stage == migrationv1alpha1.ConditionTypeCleaned {
		for _, cluster := range bundle.ManagedClusters {
			log.Infof("cleaned up the source hub resources: %s", evt.Source())
			err = db.Model(&models.ManagedClusterMigration{}).
				Where("to_hub = ?", eventClusterName).
				Where("cluster_name = ?", cluster).
				Update("stage", migrationv1alpha1.ConditionTypeCleaned).Error
			if err != nil {
				log.Errorf("failed to mark the MigrationCompleted for %s - $s in db: %v", evt.Source(), cluster, err)
				return err
			}
		}
	}
	return nil
}
