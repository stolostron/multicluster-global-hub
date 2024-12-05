// Copyright (c) 2024 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package managedcluster

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	addonv1 "github.com/stolostron/klusterlet-addon-controller/pkg/apis/agent/v1"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type klusterletAddonConfigHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	manager       ctrl.Manager
}

func RegisterKlusterletAddonConfigHandler(mgr ctrl.Manager, conflationManager *conflator.ConflationManager) {
	k := &klusterletAddonConfigHandler{
		log:           logger.ZapLogger("klusterlet-addon-config-handler"),
		eventType:     string(enum.KlusterletAddonConfigType),
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.KlusterletAddonConfigPriority,
		manager:       mgr,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		k.eventPriority,
		k.eventSyncMode,
		k.eventType,
		k.handleKlusterletAddonConfigEvent,
	))
}

func (k *klusterletAddonConfigHandler) handleKlusterletAddonConfigEvent(ctx context.Context, evt *cloudevents.Event) error {
	k.log.Debugw("handle klusterlet addon config", "cloudevents", evt)
	fmt.Println("handleKlusterletAddonConfigEvent")
	klusterletAddonConfig := &addonv1.KlusterletAddonConfig{}
	if err := evt.DataAs(klusterletAddonConfig); err != nil {
		return err
	}

	klusterletAddonConfigData, err := json.Marshal(klusterletAddonConfig)
	if err != nil {
		return err
	}

	migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := k.manager.GetClient().List(ctx, migrationList, &client.ListOptions{
		Namespace: utils.GetDefaultNamespace(),
	}); err != nil {
		return err
	}

	// update it into managedclustermigration CR
	if len(migrationList.Items) > 0 {
		migration := migrationList.Items[0]
		if len(migration.GetAnnotations()) == 0 {
			migration.Annotations = map[string]string{}
		}
		migration.Annotations[constants.KlusterletAddonConfigAnnotation] = string(klusterletAddonConfigData)
		if err := k.manager.GetClient().Update(ctx, &migration); err != nil {
			return err
		}
	}

	return nil
}
