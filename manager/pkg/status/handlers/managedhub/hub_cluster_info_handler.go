package managedhub

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.DefaultZapLogger()

type hubClusterInfoHandler struct {
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegsiterHubClusterInfoHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.HubClusterInfoType)
	hubClusterInfo := &hubClusterInfoHandler{
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
	log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

	hubInfoData := &cluster.HubClusterInfo{}
	if err := evt.DataAs(hubInfoData); err != nil {
		log.Warnw("failed to unmarshal bundle", "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(),
			"version", version, "error", err)
		return nil
	}

	leafHubName := evt.Source()

	// Ignore data which do not have clusterid
	if len(hubInfoData.ClusterId) == 0 {
		log.Infof("no cluster id for hub info, hubInfo: %v", hubInfoData)
		return nil
	}
	clusterId := hubInfoData.ClusterId
	payload, err := json.Marshal(hubInfoData)
	if err != nil {
		return err
	}

	db := database.GetGorm()
	err = db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "leaf_hub_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"payload", "updated_at"}),
	}).Create(&models.LeafHub{
		LeafHubName: leafHubName,
		ClusterID:   clusterId,
		Payload:     payload,
	}).Error
	if err != nil {
		log.Errorw("failed to upsert hubinfo", "name", leafHubName, "id", clusterId, "error", err)
		return err
	}

	log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

// TODO: Should get the cluster info by leafhub name and cluster id!
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
