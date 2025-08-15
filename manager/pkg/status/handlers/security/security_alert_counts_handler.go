package security

import (
	"context"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	dbmodels "github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	wiremodels "github.com/stolostron/multicluster-global-hub/pkg/wire/models"
)

type securityAlertCountsHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterSecurityAlertCountsHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.SecurityAlertCountsType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &securityAlertCountsHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.SecurityAlertCountsPriority,
	}
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
	h.log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

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
		Source:    wireModel.Source,
	}

	// Insert or update the data in the database:
	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "hub_name"}, {Name: "source"}},
		UpdateAll: true,
	}).Create(dbModel).Error
	if err != nil {
		return err
	}

	h.log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
