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

func (h *localRootPolicyEventHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	db := database.GetGorm()

	eventMode := evt.Extensions()[constants.CloudEventExtensionSendMode]
	if eventMode == string(constants.EventSendModeSingle) {
		h.log.Debugw("handling single root policy event", "type", evt.Type(), "LH", evt.Source(), "version", version)
		rootPolicyEvent := &event.RootPolicyEvent{}
		if err := evt.DataAs(rootPolicyEvent); err != nil {
			return err
		}

		localRootPolicyEventModel, err := convertRootPolicyEventToModel(rootPolicyEvent, leafHubName)
		if err != nil {
			return fmt.Errorf("failed to convert root policy event to model: %w", err)
		}
		err = db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
			UpdateAll: true,
		}).Create(localRootPolicyEventModel).Error
		if err != nil {
			return fmt.Errorf("failed to handle the event to database %v", err)
		}
		return nil
	}

	data := event.RootPolicyEventBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("the root policy event payload shouldn't be empty")
	}

	localRootPolicyEvents := []models.LocalRootPolicyEvent{}
	for _, rootPolicyEvent := range data {
		localRootPolicyEventModel, err := convertRootPolicyEventToModel(rootPolicyEvent, leafHubName)
		if err != nil {
			return fmt.Errorf("failed to convert root policy event to model: %w", err)
		}
		localRootPolicyEvents = append(localRootPolicyEvents, *localRootPolicyEventModel)
	}

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

func convertRootPolicyEventToModel(rootPolicyEvent *event.RootPolicyEvent, leafHubName string) (
	*models.LocalRootPolicyEvent, error,
) {
	if rootPolicyEvent.PolicyID == "" {
		return nil, fmt.Errorf("root policy event without policy ID")
	}
	sourceJSONB, err := json.Marshal(rootPolicyEvent.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the event source: %w", err)
	}
	return &models.LocalRootPolicyEvent{
		BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
			LeafHubName:    leafHubName,
			EventName:      rootPolicyEvent.EventName,
			EventNamespace: rootPolicyEvent.EventNamespace,
			PolicyID:       rootPolicyEvent.PolicyID,
			Message:        rootPolicyEvent.Message,
			Reason:         rootPolicyEvent.Reason,
			Source:         sourceJSONB,
			Count:          int(rootPolicyEvent.Count),
			Compliance:     string(common.GetDatabaseCompliance(rootPolicyEvent.Compliance, log)),
			CreatedAt:      rootPolicyEvent.CreatedAt,
		},
	}, nil
}
