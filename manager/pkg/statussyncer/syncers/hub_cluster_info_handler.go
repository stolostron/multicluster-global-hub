package dbsyncer

import (
	"context"
	"encoding/json"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

type hubClusterInfoHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode metadata.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func NewHubClusterInfoHandler() Handler {
	eventType := string(enum.HubClusterInfoType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	return &hubClusterInfoHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: metadata.CompleteStateMode,
		eventPriority: conflator.HubClusterInfoPriority,
	}
}

func (h *hubClusterInfoHandler) RegisterHandler(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler functions need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (h *hubClusterInfoHandler) handleEvent(ctx context.Context,
	evt *cloudevents.Event,
) error {
	version := evt.Extensions()[metadata.ExtVersion]
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

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

	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
