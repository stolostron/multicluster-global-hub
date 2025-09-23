package managedcluster

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type managedClusterEventHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterManagedClusterEventHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.ManagedClusterEventType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &managedClusterEventHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.DeltaStateMode,
		eventPriority: conflator.ManagedClusterEventPriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *managedClusterEventHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)
	db := database.GetGorm()

	eventMode := evt.Extensions()[constants.CloudEventExtensionSendMode]
	if eventMode == string(constants.EventSendModeSingle) {
		h.log.Debugw("handling single managed cluster event", "type", evt.Type(), "LH", evt.Source(), "version", version)
		clusterEvent := models.ManagedClusterEvent{}
		if err := evt.DataAs(&clusterEvent); err != nil {
			return err
		}
		clusterEvent.LeafHubName = leafHubName
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "leaf_hub_name"}, {Name: "event_name"}, {Name: "created_at"}},
			DoNothing: true,
		}).Create(&clusterEvent).Error
		if err != nil {
			return fmt.Errorf("failed handling single managed cluster event - %w", err)
		}
		return nil
	}

	managedClusterEvents := event.ManagedClusterEventBundle{}
	if err := evt.DataAs(&managedClusterEvents); err != nil {
		return err
	}

	for _, managedClusterEvent := range managedClusterEvents {
		managedClusterEvent.LeafHubName = leafHubName
	}

	if len(managedClusterEvents) <= 0 {
		h.log.Info("empty managed cluster event payload", "event", evt)
		return nil
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "leaf_hub_name"}, {Name: "event_name"}, {Name: "created_at"}},
		DoNothing: true,
	}).CreateInBatches(managedClusterEvents, BatchSize).Error
	if err != nil {
		return fmt.Errorf("failed handling batch managed cluster event - %w", err)
	}

	h.log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
