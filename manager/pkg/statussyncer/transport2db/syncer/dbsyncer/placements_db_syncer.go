package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewPlacementsDBSyncer creates a new instance of genericDBSyncer to sync placements.
func NewPlacementsDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.PlacementsTableName,
		createBundleFunc: bundle.NewPlacementsBundle,
		bundlePriority:   conflator.PlacementPriority,
		bundleSyncMode:   status.CompleteStateMode,
	}

	log.Info("initialized placements db syncer")

	return dbSyncer
}
