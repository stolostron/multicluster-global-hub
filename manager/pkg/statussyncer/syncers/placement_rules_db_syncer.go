package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewPlacementRulesDBSyncer creates a new instance of genericDBSyncer to sync placement-rules.
func NewPlacementRulesDBSyncer(log logr.Logger) Syncer {
	return &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.PlacementRuleMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.PlacementRulesTableName,
		createBundleFunc: placement.NewManagerPlacementRulesBundle,
		bundlePriority:   conflator.PlacementRulePriority,
		bundleSyncMode:   metadata.CompleteStateMode,
	}
}
