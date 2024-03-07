package dbsyncer

import (
	"context"
	"fmt"
	"strings"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type hubHeartbeatHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode metadata.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewHubClusterHeartbeatHandler() Handler {
	eventType := string(enum.HubClusterHeartbeatType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &hubHeartbeatHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: metadata.CompleteStateMode,
		eventPriority: conflator.HubClusterHeartbeatPriority,
	}
}

func (h *hubHeartbeatHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		handleHeartbeatEvent,
	))
}

func handleHeartbeatEvent(ctx context.Context, evt *cloudevents.Event) error {
	fmt.Println("handle heart beat ..............................")
	db := database.GetGorm()
	heartbeat := models.LeafHubHeartbeat{
		Name:         evt.Source(),
		LastUpdateAt: time.Now(),
	}
	err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&heartbeat).Error
	if err != nil {
		return fmt.Errorf("failed to update heartbeat %v", err)
	}
	return nil
}
