package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewPlacementDecisionsDBSyncer creates a new instance of genericDBSyncer to sync placement-decisions.
func NewPlacementDecisionsDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementDecisionMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.PlacementDecisionsTableName,
		createBundleFunc: statusbundle.NewPlacementDecisionsBundle,
		bundlePriority:   conflator.PlacementDecisionPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}

	log.Info("initialized placement-decisions db syncer")

	return dbSyncer
}
