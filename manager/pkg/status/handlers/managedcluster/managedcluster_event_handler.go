package managedcluster

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type managedClusterEventHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterManagedClusterEventHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.ManagedClusterEventType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &managedClusterEventHandler{
		log:           ctrl.Log.WithName(logName),
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
	h.log.V(2).Info("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

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

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "leaf_hub_name"}, {Name: "event_name"}, {Name: "created_at"}},
		DoNothing: true,
	}).CreateInBatches(managedClusterEvents, 100).Error
	if err != nil {
		return fmt.Errorf("failed handling leaf hub LocalPolicyStatusEvent event - %w", err)
	}

	h.log.V(2).Info("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
