package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewSubscriptionStatusesDBSyncer creates a new instance of genericDBSyncer to sync subscription-statuses.
func NewSubscriptionStatusesDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionStatusMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.SubscriptionStatusesTableName,
		createBundleFunc: statusbundle.NewSubscriptionStatusesBundle,
		bundlePriority:   conflator.SubscriptionStatusPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}

	log.Info("initialized subscription-statuses db syncer")

	return dbSyncer
}
