package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewPlacementsDBSyncer creates a new instance of genericDBSyncer to sync placements.
func NewPlacementsDBSyncer(log logr.Logger) Syncer {
	dbSyncer := &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.PlacementsTableName,
		createBundleFunc: placement.NewManagerPlacementsBundle,
		bundlePriority:   conflator.PlacementPriority,
		bundleSyncMode:   metadata.CompleteStateMode,
	}

	log.Info("initialized placements db syncer")

	return dbSyncer
}
