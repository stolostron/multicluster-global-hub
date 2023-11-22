package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewSubscriptionStatusesDBSyncer creates a new instance of genericDBSyncer to sync subscription-statuses.
func NewSubscriptionStatusesDBSyncer(log logr.Logger) Syncer {
	dbSyncer := &genericStatusSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionStatusMsgKey,
		dbSchema:         database.StatusSchema,
		dbTableName:      database.SubscriptionStatusesTableName,
		createBundleFunc: statusbundle.NewSubscriptionStatusesBundle,
		bundlePriority:   conflator.SubscriptionStatusPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}

	log.Info("initialized subscription-statuses db syncer")

	return dbSyncer
}
