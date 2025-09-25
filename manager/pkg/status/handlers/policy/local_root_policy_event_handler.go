package policy

import (
	"context"
	"encoding/json"
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
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type localRootPolicyEventHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterLocalRootPolicyEventHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalRootPolicyEventType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &localRootPolicyEventHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.DeltaStateMode,
		eventPriority: conflator.LocalEventRootPolicyPriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

// convertEventToModel converts a single root policy event to database model
func (h *localRootPolicyEventHandler) convertEventToModel(element *event.RootPolicyEvent, leafHubName string) (*models.LocalRootPolicyEvent, error) {
	if element.PolicyID == "" {
		return nil, fmt.Errorf("policy ID cannot be empty")
	}

	sourceJSONB, err := json.Marshal(element.Source)
	if err != nil {
		h.log.Error(err, "failed to parse the event source", "source", element.Source)
		return nil, err
	}

	return &models.LocalRootPolicyEvent{
		BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
			LeafHubName:    leafHubName,
			EventName:      element.EventName,
			EventNamespace: element.EventNamespace,
			PolicyID:       element.PolicyID,
			Message:        element.Message,
			Reason:         element.Reason,
			Source:         sourceJSONB,
			Count:          int(element.Count),
			Compliance:     string(common.GetDatabaseCompliance(element.Compliance, h.log)),
			CreatedAt:      element.CreatedAt,
		},
	}, nil
}

func (h *localRootPolicyEventHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	// Check if this is a single event mode
	if eventMode, exists := evt.Extensions()[constants.CloudEventExtensionSendMode]; exists {
		if eventMode == string(constants.EventSendModeSingle) {
			// Handle single event
			singleEvent := &event.RootPolicyEvent{}
			if err := evt.DataAs(singleEvent); err != nil {
				return err
			}

			localEvent, err := h.convertEventToModel(singleEvent, leafHubName)
			if err != nil {
				return err
			}

			db := database.GetGorm()
			err = db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
				UpdateAll: true,
			}).Create(localEvent).Error
			if err != nil {
				return fmt.Errorf("failed handling single root policy event - %w", err)
			}

			h.log.Debugw("single event handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
			return nil
		}
	}

	// Handle batch events (existing logic)
	data := event.RootPolicyEventBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("the root policy event payload shouldn't be empty")
	}

	localRootPolicyEvents := []models.LocalRootPolicyEvent{}
	for _, element := range data {
		localEvent, err := h.convertEventToModel(element, leafHubName)
		if err != nil {
			h.log.Error(err, "failed to convert event to model")
			continue
		}
		localRootPolicyEvents = append(localRootPolicyEvents, *localEvent)
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		UpdateAll: true,
	}).CreateInBatches(localRootPolicyEvents, 100).Error
	if err != nil {
		return fmt.Errorf("failed to handle the event to database %v", err)
	}
	h.log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
