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

	data := event.ReplicatedPolicyEventBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	batchLocalPolicyEvents := []models.LocalReplicatedPolicyEvent{}
	for _, policyStatusEvent := range data {

		sourceJSONB, err := json.Marshal(policyStatusEvent.Source)
		if err != nil {
			h.log.Error(err, "failed to parse the event source", "source", policyStatusEvent.Source)
		}

		batchLocalPolicyEvents = append(batchLocalPolicyEvents, models.LocalReplicatedPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:      policyStatusEvent.EventName,
				EventNamespace: policyStatusEvent.EventNamespace,
				PolicyID:       policyStatusEvent.PolicyID,
				Message:        policyStatusEvent.Message,
				Reason:         policyStatusEvent.Reason,
				LeafHubName:    leafHubName,
				Source:         sourceJSONB,
				Count:          int(policyStatusEvent.Count),
				Compliance:     string(common.GetDatabaseCompliance(policyStatusEvent.Compliance, h.log)),
				CreatedAt:      policyStatusEvent.CreatedAt.Time,
			},
			ClusterID:   policyStatusEvent.ClusterID,
			ClusterName: policyStatusEvent.ClusterName,
		})
	}

	if len(batchLocalPolicyEvents) <= 0 {
		return nil
	}

	db := database.GetGorm()
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
