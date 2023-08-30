package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// NewHubClusterInfoDBSyncer creates a new instance of genericDBSyncer to sync hub cluster info.
func NewHubClusterInfoDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &hubClusterInfoDBSyncer{
		log: log,
		createHubClusterInfoFunc: func() status.Bundle {
			return &status.BaseLeafHubClusterInfoStatusBundle{}
		},
	}

	log.Info("initialized hub cluster info db syncer")

	return dbSyncer
}

// localSpecPoliciesSyncer implements local objects spec db sync business logic.
type hubClusterInfoDBSyncer struct {
	log                      logr.Logger
	createHubClusterInfoFunc status.CreateBundleFunction
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
		conflator.HubClusterInfoStatusPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createHubClusterInfoFunc()),
		syncer.handleLocalObjectsBundleWrapper(database.HubClusterInfoTableName)))
}

func (syncer *hubClusterInfoDBSyncer) handleLocalObjectsBundleWrapper(tableName string) func(ctx context.Context,
	bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
	return func(ctx context.Context, bundle status.Bundle,
		dbClient postgres.StatusTransportBridgeDB,
	) error {
		return syncer.handleLocalObjectsBundle(ctx, bundle, dbClient, database.LocalSpecSchema, tableName)
	}
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
func (syncer *hubClusterInfoDBSyncer) handleLocalObjectsBundle(ctx context.Context, bundle status.Bundle,
	dbClient postgres.LocalPoliciesStatusDB, schema string, tableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	leafHub := &models.LeafHub{}
	db := database.GetGorm()
	err := db.Where(&models.LeafHub{
		LeafHubName: leafHubName,
	}).Find(leafHub).Error
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s.%s' from db - %w", schema, tableName, err)
	}

	// https://gorm.io/docs/transactions.html
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, object := range bundle.GetObjects() {
			specificObj, ok := object.(*status.LeafHubClusterInfo)
			if !ok {
				continue
			}

			payload, err := json.Marshal(specificObj)
			if err != nil {
				return err
			}

			// if the row doesn't exist in db then add it.
			if leafHub.LeafHubName == "" {
				err = tx.Create(&models.LeafHub{
					LeafHubName: leafHubName,
					Payload:     payload,
				}).Error
				if err != nil {
					return err
				}
				continue
			}

			err = tx.Model(&models.LeafHub{}).
				Where(&models.LeafHub{
					LeafHubName: leafHubName,
				}).
				Updates(models.LeafHub{
					Payload:     payload,
					LeafHubName: leafHubName,
				}).Error
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed handling leaf hub '%s.%s' bundle - %w", schema, tableName, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}
