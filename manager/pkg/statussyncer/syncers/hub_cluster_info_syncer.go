package dbsyncer

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/base"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

// hubClusterInfoDBSyncer implements local objects spec db sync business logic.
type hubClusterInfoDBSyncer struct {
	log                      logr.Logger
	createHubClusterInfoFunc CreateBundleFunction
}

// NewHubClusterInfoDBSyncer creates a new instance of genericDBSyncer to sync hub cluster info.
func NewHubClusterInfoDBSyncer(log logr.Logger) Syncer {
	return &hubClusterInfoDBSyncer{
		log: log,
		createHubClusterInfoFunc: func() bundle.ManagerBundle {
			return &cluster.HubClusterInfoBundle{}
		},
	}
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *hubClusterInfoDBSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.HubClusterInfoMsgKey,
		CreateBundleFunc: syncer.createHubClusterInfoFunc,
		Predicate:        func() bool { return true }, // always get hub info bundles
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler functions need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *hubClusterInfoDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.HubClusterInfoPriority,
		metadata.CompleteStateMode,
		bundle.GetBundleType(syncer.createHubClusterInfoFunc()),
		syncer.handleLocalObjectsBundleWrapper()))
}

func (syncer *hubClusterInfoDBSyncer) handleLocalObjectsBundleWrapper() func(ctx context.Context,
	bundle bundle.ManagerBundle) error {
	return func(ctx context.Context, bundle bundle.ManagerBundle) error {
		return syncer.handleLocalObjectsBundle(ctx, bundle)
	}
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
func (syncer *hubClusterInfoDBSyncer) handleLocalObjectsBundle(ctx context.Context, bundle bundle.ManagerBundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	existingObjs := []models.LeafHub{}
	db := database.GetGorm()

	// We use gorm soft delete: https://gorm.io/gen/delete.html#Soft-Delete
	// So, the db query will not get deleted leafhubs, then we could use leafhub name to identy the unique leafhub
	err := db.Where(&models.LeafHub{
		LeafHubName: leafHubName,
	}).Find(&existingObjs).Error
	if err != nil {
		return err
	}

	// It should only have one item in the objects
	for _, object := range bundle.GetObjects() {
		specificObj, ok := object.(*base.HubClusterInfo)
		if !ok {
			continue
		}

		// Handle agent version is 1.0 and manager version is 1.1 or bigger
		clusterId := constants.DefaultClusterId
		if len(specificObj.ClusterId) != 0 {
			clusterId = specificObj.ClusterId
		}

		payload, err := json.Marshal(specificObj)
		if err != nil {
			return err
		}

		syncer.log.V(2).Info("Existing objs", "existingObjs", existingObjs)
		if len(existingObjs) == 0 {
			err := db.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "cluster_id"}, {Name: "leaf_hub_name"}},
				UpdateAll: true,
			}).Create(&models.LeafHub{
				LeafHubName: leafHubName,
				ClusterID:   clusterId,
				Payload:     payload,
			}).Error
			if err != nil {
				syncer.log.Error(err, "failed to upinsert hubinfo", "name", leafHubName, "id", clusterId)
			}
			return err
		}
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
			syncer.log.Error(err, "failed to update the existing hubinfo", "name", leafHubName, "id", clusterId)
			return err
		}
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}
