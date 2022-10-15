package dbsyncer

import (
	"github.com/go-logr/logr"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewSubscriptionReportsDBSyncer creates a new instance of genericDBSyncer to sync subscription-reports.
func NewSubscriptionReportsDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &genericDBSyncer{
		log:              log,
		transportMsgKey:  constants.SubscriptionReportMsgKey,
		dbSchema:         db.StatusSchema,
		dbTableName:      db.SubscriptionReportsTableName,
		createBundleFunc: statusbundle.NewSubscriptionReportsBundle,
		bundlePriority:   conflator.SubscriptionReportPriority,
		bundleSyncMode:   bundle.CompleteStateMode,
	}

	log.Info("initialized subscription-reports db syncer")

	return dbSyncer
}
