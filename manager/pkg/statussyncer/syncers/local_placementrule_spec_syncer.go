package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/placement"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewLocalSpecPlacementruleSyncer creates a new instance of LocalSpecDBSyncer.
func NewLocalSpecPlacementruleSyncer(log logr.Logger) Syncer {
	return &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.LocalPlacementRulesMsgKey,
		dbSchema:         database.LocalSpecSchema,
		dbTableName:      database.PlacementRulesTableName,
		createBundleFunc: placement.NewManagerLocalPlacementRulesBundle,
		bundlePriority:   conflator.LocalPlacementRulesSpecPriority,
		bundleSyncMode:   metadata.CompleteStateMode,
	}
}
