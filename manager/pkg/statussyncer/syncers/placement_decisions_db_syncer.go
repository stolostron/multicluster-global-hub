package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewPlacementDecisionsDBSyncer creates a new instance of genericDBSyncer to sync placement-decisions.
func NewPlacementDecisionsDBSyncer(log logr.Logger) Syncer {
	dbSyncer := &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementDecisionMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.PlacementDecisionsTableName,
		createBundleFunc: placement.NewManagerPlacementDecisionsBundle,
		bundlePriority:   conflator.PlacementDecisionPriority,
		bundleSyncMode:   metadata.CompleteStateMode,
	}

	log.Info("initialized placement-decisions db syncer")
	return dbSyncer
}
