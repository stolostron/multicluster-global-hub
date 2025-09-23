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

type localReplicatedPolicyEventHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterLocalReplicatedPolicyEventHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalReplicatedPolicyEventType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &localReplicatedPolicyEventHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.DeltaStateMode,
		eventPriority: conflator.LocalReplicatedPolicyEventPriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *localReplicatedPolicyEventHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	db := database.GetGorm()

	eventMode := evt.Extensions()[constants.CloudEventExtensionSendMode]
	if eventMode == string(constants.EventSendModeSingle) {
		h.log.Debugw("handling single replicated policy event", "type", evt.Type(), "LH", evt.Source(), "version", version)
		policyStatusEvent := &event.ReplicatedPolicyEvent{}
		if err := evt.DataAs(policyStatusEvent); err != nil {
			return err
		}
		model, err := convertReplicatedPolicyEventToModel(policyStatusEvent, leafHubName)
		if err != nil {
			return fmt.Errorf("failed to convert replicated policy event to model: %w", err)
		}
		err = db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
			UpdateAll: true,
		}).Create(model).Error
		if err != nil {
			return fmt.Errorf("failed to handle the event to database %v", err)
		}
		return nil
	}

	data := event.ReplicatedPolicyEventBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	batchLocalPolicyEvents := []models.LocalReplicatedPolicyEvent{}
	for _, policyStatusEvent := range data {

		model, err := convertReplicatedPolicyEventToModel(policyStatusEvent, leafHubName)
		if err != nil {
			return fmt.Errorf("failed to convert replicated policy event to model: %w", err)
		}
		batchLocalPolicyEvents = append(batchLocalPolicyEvents, *model)
	}

	if len(batchLocalPolicyEvents) <= 0 {
		return nil
	}

	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		DoNothing: true,
	}).CreateInBatches(batchLocalPolicyEvents, 100).Error
	if err != nil {
		return fmt.Errorf("failed handling leaf hub LocalPolicyStatusEvent event - %w", err)
	}

	h.log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func convertReplicatedPolicyEventToModel(replicatedPolicyEvent *event.ReplicatedPolicyEvent, leafHubName string) (
	*models.LocalReplicatedPolicyEvent, error,
) {
	sourceJSONB, err := json.Marshal(replicatedPolicyEvent.Source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse the event source: %w", err)
	}
	return &models.LocalReplicatedPolicyEvent{
		BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
			EventName:      replicatedPolicyEvent.EventName,
			EventNamespace: replicatedPolicyEvent.EventNamespace,
			PolicyID:       replicatedPolicyEvent.PolicyID,
			Message:        replicatedPolicyEvent.Message,
			Reason:         replicatedPolicyEvent.Reason,
			LeafHubName:    leafHubName,
			Source:         sourceJSONB,
			Count:          int(replicatedPolicyEvent.Count),
			Compliance:     string(common.GetDatabaseCompliance(replicatedPolicyEvent.Compliance, log)),
			CreatedAt:      replicatedPolicyEvent.CreatedAt,
		},
		ClusterID:   replicatedPolicyEvent.ClusterID,
		ClusterName: replicatedPolicyEvent.ClusterName,
	}, nil
}
