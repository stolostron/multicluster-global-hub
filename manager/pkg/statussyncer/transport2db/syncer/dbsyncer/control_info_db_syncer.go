package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// NewControlInfoDBSyncer creates a new instance of ControlInfoDBSyncer.
func NewControlInfoDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &ControlInfoDBSyncer{
		log:              log,
		createBundleFunc: statusbundle.NewControlInfoBundle,
	}

	log.Info("initialized control info db syncer")

	return dbSyncer
}

// ControlInfoDBSyncer implements control info transport to db sync.
type ControlInfoDBSyncer struct {
	log              logr.Logger
	createBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *ControlInfoDBSyncer) RegisterCreateBundleFunctions(transportInstance transport.Transport) {
	transportInstance.BundleRegister(&registration.BundleRegistration{
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
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleControlInfoBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *ControlInfoDBSyncer) handleControlInfoBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.ControlInfoDB,
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
