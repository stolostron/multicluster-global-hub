package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewHubClusterHeartbeatDBSyncer creates a new instance of ControlInfoDBSyncer.
func NewHubClusterHeartbeatDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &HubClusterHeartbeatDBSyncer{
		log:              log,
		createBundleFunc: statusbundle.NewHubClusterInfoBundle,
	}

	log.Info("initialized control info db syncer")

	return dbSyncer
}

// HubClusterHeartbeatDBSyncer implements control info transport to db sync.
type HubClusterHeartbeatDBSyncer struct {
	log              logr.Logger
	createBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *HubClusterHeartbeatDBSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.ControlInfoMsgKey,
		CreateBundleFunc: syncer.createBundleFunc,
		Predicate:        func() bool { return true }, // always get control info bundles
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
func (syncer *HubClusterHeartbeatDBSyncer) RegisterBundleHandlerFunctions(
	conflationManager *conflator.ConflationManager,
) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.ControlInfoPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleControlInfoBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *HubClusterHeartbeatDBSyncer) handleControlInfoBundle(ctx context.Context, bundle status.Bundle,
	dbClient postgres.ControlInfoDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	if err := dbClient.UpdateHeartbeat(ctx, database.StatusSchema,
		database.LeafHubHeartbeatsTableName, leafHubName); err != nil {
		return fmt.Errorf("failed handling control info bundle of leaf hub '%s' - %w", leafHubName, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}
