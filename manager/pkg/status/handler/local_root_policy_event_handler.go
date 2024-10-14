package handler

import (
	"context"
	"encoding/json"
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
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type localRootPolicyEventHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewLocalRootPolicyEventHandler() conflator.Handler {
	eventType := string(enum.LocalRootPolicyEventType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &localRootPolicyEventHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: enum.DeltaStateMode,
		eventPriority: conflator.LocalEventRootPolicyPriority,
	}
}

func (h *localRootPolicyEventHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
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
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	data := []event.RootPolicyEvent{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}
	if len(data) == 0 {
		return fmt.Errorf("the root policy event payload shouldn't be empty")
	}

	localRootPolicyEvents := []models.LocalRootPolicyEvent{}
	for _, element := range data {
		if element.PolicyID == "" {
			continue
		}

		sourceJSONB, err := json.Marshal(element.Source)
		if err != nil {
			h.log.Error(err, "failed to parse the event source", "source", element.Source)
		}
		localRootPolicyEvents = append(localRootPolicyEvents, models.LocalRootPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				LeafHubName:    leafHubName,
				EventName:      element.EventName,
				EventNamespace: element.EventNamespace,
				PolicyID:       element.PolicyID,
				Message:        element.Message,
				Reason:         element.Reason,
				Source:         sourceJSONB,
				Count:          int(element.Count),
				Compliance:     string(common.GetDatabaseCompliance(element.Compliance)),
				CreatedAt:      element.CreatedAt.Time,
			},
		})
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		UpdateAll: true,
	}).CreateInBatches(localRootPolicyEvents, 100).Error
	if err != nil {
		return fmt.Errorf("failed to handle the event to database %v", err)
	}
	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
