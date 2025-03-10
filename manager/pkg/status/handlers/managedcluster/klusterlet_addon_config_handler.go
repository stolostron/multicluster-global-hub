// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"encoding/json"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/types"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type klusterletAddonConfigHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	client        client.Client
}

func RegisterKlusterletAddonConfigHandler(mgr ctrl.Manager, conflationManager *conflator.ConflationManager) {
	k := &klusterletAddonConfigHandler{
		log:           logger.ZapLogger("klusterlet-addon-config-handler"),
		eventType:     string(enum.KlusterletAddonConfigType),
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.KlusterletAddonConfigPriority,
		client:        mgr.GetClient(),
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		k.eventPriority,
		k.eventSyncMode,
		k.eventType,
		k.handleKlusterletAddonConfigEvent,
	))
}

func (k *klusterletAddonConfigHandler) handleKlusterletAddonConfigEvent(ctx context.Context,
	evt *cloudevents.Event,
) error {
	k.log.Debugw("handle klusterlet addon config", "cloudevents", evt)
	klusterletAddonConfig := &addonv1.KlusterletAddonConfig{}
	if err := evt.DataAs(klusterletAddonConfig); err != nil {
		return err
	}

	klusterletAddonConfigData, err := json.Marshal(klusterletAddonConfig)
	if err != nil {
		return err
	}

	toHub, err := types.ToString(evt.Extensions()[constants.CloudEventExtensionKeyClusterName])
	if err != nil {
		k.log.Warn("failed to parse migration to hub from event", "error", err)
	}

	// LeafHubName
	fromHub := evt.Source()

	cluster := klusterletAddonConfig.Name

	mcm := models.ManagedClusterMigration{
		FromHub:     fromHub,
		ToHub:       toHub,
		ClusterName: cluster,
		Payload:     klusterletAddonConfigData,
		Stage:       migrationv1alpha1.PhaseInitializing,
	}

	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "from_hub"}, {Name: "to_hub"}, {Name: "cluster_name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"payload": klusterletAddonConfigData,
		}),
	}).Create(&mcm).Error
	if err != nil {
		k.log.Errorf("failed to update the migration initializing data into db: %v", err)
		return err
	}

	return nil
}
