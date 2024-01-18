package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

type hubHeartbeatSyncer struct {
	log                 logr.Logger
	heartbeatBundleFunc CreateBundleFunction
}

func NewHubClusterHeartbeatSyncer(log logr.Logger) Syncer {
	return &hubHeartbeatSyncer{
		log:                 log,
		heartbeatBundleFunc: cluster.NewManagerHubClusterHeartbeatBundle,
	}
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *hubHeartbeatSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.HubClusterHeartbeatMsgKey,
		CreateBundleFunc: syncer.heartbeatBundleFunc,
		Predicate:        func() bool { return true }, // always get hub info bundles
	})
}

func (syncer *hubHeartbeatSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.HubClusterHeartbeatPriority,
		metadata.CompleteStateMode,
		bundle.GetBundleType(syncer.heartbeatBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper()))
}

func (syncer *hubHeartbeatSyncer) handleLocalObjectsBundleWrapper() func(ctx context.Context,
	bundle bundle.ManagerBundle) error {
	return func(ctx context.Context, bundle bundle.ManagerBundle) error {
		return handleHeartbeatBundle(ctx, bundle)
	}
}

func handleHeartbeatBundle(ctx context.Context, bundle bundle.ManagerBundle) error {
	db := database.GetGorm()

	heartbeat := models.LeafHubHeartbeat{
		Name:         bundle.GetLeafHubName(),
		LastUpdateAt: time.Now(),
	}

	err := db.Model(&heartbeat).Clauses(clause.OnConflict{UpdateAll: true}).Create(&heartbeat)
	if err != nil {
		return fmt.Errorf("failed to update heartbeat %v", err)
	}

	return nil
}
