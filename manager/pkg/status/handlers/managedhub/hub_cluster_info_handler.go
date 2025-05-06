package managedhub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type hubClusterInfoHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegsiterHubClusterInfoHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.HubClusterInfoType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	hubClusterInfo := &hubClusterInfoHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.HubClusterInfoPriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		hubClusterInfo.eventPriority,
		hubClusterInfo.eventSyncMode,
		hubClusterInfo.eventType,
		hubClusterInfo.handleEvent,
	))
}

func (h *hubClusterInfoHandler) handleEvent(ctx context.Context,
	evt *cloudevents.Event,
) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	h.log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

	leafHubName := evt.Source()

	db := database.GetGorm()

	// We use gorm soft delete: https://gorm.io/gen/delete.html#Soft-Delete
	// So, the db query will not get deleted leafhubs, then we could use leafhub name to identy the unique leafhub
	existingObjects := []models.LeafHub{}
	err := db.Where(&models.LeafHub{LeafHubName: leafHubName}).Find(&existingObjects).Error
	if err != nil {
		return err
	}

	hubInfoData := &cluster.HubClusterInfo{}
	if err := evt.DataAs(hubInfoData); err != nil {
		return err
	}

	// Handle agent version is 1.0 and manager version is 1.1 or bigger
	clusterId := constants.DefaultClusterId
	if len(hubInfoData.ClusterId) != 0 {
		clusterId = hubInfoData.ClusterId
	}
	payload, err := json.Marshal(hubInfoData)
	if err != nil {
		return err
	}

	// create
	if len(existingObjects) == 0 {
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "leaf_hub_name"}},
			UpdateAll: true,
		}).Create(&models.LeafHub{
			LeafHubName: leafHubName,
			ClusterID:   clusterId,
			Payload:     payload,
		}).Error
		if err != nil {
			h.log.Error(err, "failed to upinsert hubinfo", "name", leafHubName, "id", clusterId)
		}
		return err
	}

	// update
	err = db.Model(&models.LeafHub{}).
		Where(&models.LeafHub{
			LeafHubName: leafHubName,
		}).
		Updates(&models.LeafHub{
			LeafHubName: leafHubName,
			ClusterID:   clusterId,
			Payload:     payload,
		}).Error
	if err != nil {
		h.log.Error(err, "failed to update hubinfo", "name", leafHubName, "id", clusterId)
		return err
	}

	h.log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func GetClusterInfo(db *gorm.DB, clusterName string) (models.ClusterInfo, error) {
	var clusterInfo []models.ClusterInfo
	if db == nil {
		return models.ClusterInfo{}, fmt.Errorf("db is nil")
	}
	err := db.Select("payload->>'consoleURL' AS console_url, payload->>'mchVersion' AS mch_version").
		Where(&models.LeafHub{
			LeafHubName: clusterName,
		}).Find(&models.LeafHub{}).Scan(&clusterInfo).Error
	if err != nil {
		return models.ClusterInfo{}, err
	}
	if len(clusterInfo) == 0 {
		return models.ClusterInfo{}, fmt.Errorf("no cluster info found for %s", clusterName)
	}
	return clusterInfo[0], nil
}
