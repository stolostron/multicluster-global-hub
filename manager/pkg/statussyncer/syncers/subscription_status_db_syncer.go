package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/subscription"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewSubscriptionStatusesDBSyncer creates a new instance of genericDBSyncer to sync subscription-statuses.
func NewSubscriptionStatusesDBSyncer(log logr.Logger) Syncer {
	return &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionStatusMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.SubscriptionStatusesTableName,
		createBundleFunc: subscription.NewManagerSubscriptionStatusesBundle,
		bundlePriority:   conflator.SubscriptionStatusPriority,
		bundleSyncMode:   metadata.CompleteStateMode,
	}
}
