package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/helpers"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewControlInfoDBSyncer creates a new instance of ControlInfoDBSyncer.
func NewControlInfoDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &ControlInfoDBSyncer{
		log:              log,
		createBundleFunc: bundle.NewControlInfoBundle,
	}

	log.Info("initialized control info db syncer")

	return dbSyncer
}

// ControlInfoDBSyncer implements control info transport to db sync.
type ControlInfoDBSyncer struct {
	log              logr.Logger
	createBundleFunc bundle.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *ControlInfoDBSyncer) RegisterCreateBundleFunctions(transportInstance transport.Transport) {
	transportInstance.Register(&transport.BundleRegistration{
		MsgID:            constants.ControlInfoMsgKey,
		CreateBundleFunc: syncer.createBundleFunc,
		Predicate:        func() bool { return true }, // always get control info bundles
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
func (syncer *ControlInfoDBSyncer) RegisterBundleHandlerFunctions(
	conflationManager *conflator.ConflationManager,
) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.ControlInfoPriority,
		status.CompleteStateMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle bundle.Bundle, dbClient db.StatusTransportBridgeDB) error {
			return syncer.handleControlInfoBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *ControlInfoDBSyncer) handleControlInfoBundle(ctx context.Context, bundle bundle.Bundle,
	dbClient db.ControlInfoDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	if err := dbClient.UpdateHeartbeat(ctx, db.StatusSchema, db.LeafHubHeartbeatsTableName, leafHubName); err != nil {
		return fmt.Errorf("failed handling control info bundle of leaf hub '%s' - %w", leafHubName, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}
