package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
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
		createBundleFunc: statusbundle.NewPlacementsBundle,
		bundlePriority:   conflator.PlacementPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}

	log.Info("initialized placements db syncer")

	return dbSyncer
}
