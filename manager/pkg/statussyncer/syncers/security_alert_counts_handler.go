package dbsyncer

import (
	"context"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	dbmodels "github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	wiremodels "github.com/stolostron/multicluster-global-hub/pkg/wire/models"
)

type securityAlertCountsHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewSecurityAlertCountsHandler() conflator.Handler {
	eventType := string(enum.SecurityAlertCountsType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &securityAlertCountsHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.SecurityAlertCountsPriority,
	}
}

func (h *securityAlertCountsHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *securityAlertCountsHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	// Extract the data from the event:
	wireModel := &wiremodels.SecurityAlertCounts{}
	if err := evt.DataAs(&wireModel); err != nil {
		return err
	}

	// Convert the wire representation to the database representation. In this particular case
	// it adds the hub name and copies the wire data.
	dbModel := &dbmodels.SecurityAlertCounts{
		HubName:   leafHubName,
		Low:       wireModel.Low,
		Medium:    wireModel.Medium,
		High:      wireModel.High,
		Critical:  wireModel.Critical,
		DetailURL: wireModel.DetailURL,
	}

	// Insert or update the data in the database:
	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(dbModel).Error
	if err != nil {
		return err
	}

	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
