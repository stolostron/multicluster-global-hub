package dbsyncer

import (
	"github.com/go-logr/logr"

	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/hub-of-hubs/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/hub-of-hubs/pkg/bundle/status"
	"github.com/stolostron/hub-of-hubs/pkg/constants"
)

// NewSubscriptionStatusesDBSyncer creates a new instance of genericDBSyncer to sync subscription-statuses.
func NewSubscriptionStatusesDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionStatusMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.SubscriptionStatusesTableName,
		createBundleFunc: bundle.NewSubscriptionStatusesBundle,
		bundlePriority:   conflator.SubscriptionStatusPriority,
		bundleSyncMode:   status.CompleteStateMode,
	}

	log.Info("initialized subscription-statuses db syncer")

	return dbSyncer
}
