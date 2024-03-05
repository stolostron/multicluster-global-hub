package dbsyncer

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type localEventPolicyHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode metadata.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewLocalEventPolicyHandler() Handler {
	eventType := string(enum.LocalRootPolicyEventType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &localEventPolicyHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: metadata.CompleteStateMode,
		eventPriority: conflator.LocalEventRootPolicyPriority,
	}
}

func (h *localEventPolicyHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *localEventPolicyHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[metadata.ExtVersion]
	leafHubName := evt.Source()
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	data := []event.RootPolicyEvent{}
	if err := evt.DataAs(data); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("the root policy event payload shouldn't be empty")
	}

	localRootPolicyEvent := []models.LocalRootPolicyEvent{}
	for _, element := range data {
		if element.PolicyID == "" {
			continue
		}
		localRootPolicyEvent = append(localRootPolicyEvent, models.LocalRootPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				LeafHubName: leafHubName,
				EventName:   element.EventName,
				PolicyID:    element.PolicyID,
				Message:     element.Message,
				Reason:      element.Reason,
				Count:       int(element.Count),
				Compliance:  string(common.GetDatabaseCompliance(element.Compliance)),
				CreatedAt:   element.CreatedAt.Time,
			},
		})
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		UpdateAll: true,
	}).CreateInBatches(localRootPolicyEvent, 100).Error
	if err != nil {
		return fmt.Errorf("failed to handle the event to database %v", err)
	}
	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
