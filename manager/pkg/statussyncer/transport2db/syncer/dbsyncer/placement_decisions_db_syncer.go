package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/hub-of-hubs/pkg/bundle/status"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

// NewPlacementDecisionsDBSyncer creates a new instance of genericDBSyncer to sync placement-decisions.
func NewPlacementDecisionsDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementDecisionMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.PlacementDecisionsTableName,
		createBundleFunc: bundle.NewPlacementDecisionsBundle,
		bundlePriority:   conflator.PlacementDecisionPriority,
		bundleSyncMode:   status.CompleteStateMode,
	}

	log.Info("initialized placement-decisions db syncer")

	return dbSyncer
}
