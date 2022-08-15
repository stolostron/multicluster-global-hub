package dbsyncer

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	subscriptionv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/transport"
)

const (
	subscriptionsTableName = "subscriptions"
	subscriptionMsgKey     = "Subscriptions"
)

// AddSubscriptionsDBToTransportSyncer adds subscriptions db to transport syncer to the manager.
func AddSubscriptionsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &subscriptionv1.Subscription{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("subscriptions-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, subscriptionMsgKey, specDB, subscriptionsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add subscriptions db to transport syncer - %w", err)
	}

	return nil
}
