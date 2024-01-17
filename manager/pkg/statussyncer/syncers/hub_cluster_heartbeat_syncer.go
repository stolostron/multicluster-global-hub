package dbsyncer

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/hubmanagement"
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

func handleHeartbeatBundle(ctx context.Context, bundle bundle.ManagerBundle,
) error {
	leafHubName := bundle.GetLeafHubName()

	heartbeat := models.LeafHubHeartbeat{
		Name:         leafHubName,
		LastUpdateAt: time.Now(),
		Status:       hubmanagement.HubActive,
	}

	db := database.GetGorm()

	ret := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&heartbeat)
	return ret.Error
}
