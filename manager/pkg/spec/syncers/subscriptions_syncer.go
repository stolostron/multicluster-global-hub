package syncers

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	subscriptionsTableName = "subscriptions"
	subscriptionMsgKey     = "Subscriptions"
)

// AddSubscriptionsDBToTransportSyncer adds subscriptions db to transport syncer to the manager.
func AddSubscriptionsDBToTransportSyncer(mgr ctrl.Manager, specDB specdb.SpecDB, producer transport.Producer,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &subscriptionv1.Subscription{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            logger.ZapLogger("db-to-transport-syncer-subscriptions"),
		intervalPolicy: interval.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, producer, subscriptionMsgKey, specDB, subscriptionsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add subscriptions db to transport syncer - %w", err)
	}

	return nil
}
