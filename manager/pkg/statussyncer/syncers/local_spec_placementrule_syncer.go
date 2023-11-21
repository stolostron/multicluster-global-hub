package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
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
		createBundleFunc: statusbundle.NewLocalPlacementRulesBundle,
		bundlePriority:   conflator.LocalPlacementRulesSpecPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}
}
