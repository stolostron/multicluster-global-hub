package managedhub

import (
	"context"
	"fmt"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func RegisterHubClusterHeartbeatHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.HubClusterHeartbeatPriority,
		enum.CompleteStateMode,
		string(enum.HubClusterHeartbeatType),
		handleHeartbeatEvent,
	))
}

func handleHeartbeatEvent(ctx context.Context, evt *cloudevents.Event) error {
	db := database.GetGorm()
	heartbeat := models.LeafHubHeartbeat{
		Name:         evt.Source(),
		Status:       "active",
		LastUpdateAt: time.Now(),
	}
	err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&heartbeat).Error
	if err != nil {
		return fmt.Errorf("failed to update heartbeat %v", err)
	}
	return nil
}
