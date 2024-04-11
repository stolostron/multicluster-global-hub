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
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type localPolicyEventHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewLocalPolicyEventHandler() conflator.Handler {
	eventType := string(enum.LocalReplicatedPolicyEventType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &localPolicyEventHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: enum.DeltaStateMode,
		eventPriority: conflator.LocalReplicatedPolicyEventPriority,
	}
}

func (h *localPolicyEventHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *localPolicyEventHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	data := event.ReplicatedPolicyEventBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	batchLocalPolicyEvents := []models.LocalClusterPolicyEvent{}
	for _, policyStatusEvent := range data {
		batchLocalPolicyEvents = append(batchLocalPolicyEvents, models.LocalClusterPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:   policyStatusEvent.EventName,
				PolicyID:    policyStatusEvent.PolicyID,
				Message:     policyStatusEvent.Message,
				Reason:      policyStatusEvent.Reason,
				LeafHubName: leafHubName,
				Source:      nil,
				Count:       int(policyStatusEvent.Count),
				Compliance:  string(common.GetDatabaseCompliance(policyStatusEvent.Compliance)),
				CreatedAt:   policyStatusEvent.CreatedAt.Time,
			},
			ClusterID: policyStatusEvent.ClusterID,
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

	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
