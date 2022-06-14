package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/bundle"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/transport"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	channelv1 "open-cluster-management.io/multicloud-operators-channel/pkg/apis/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	channelsTableName = "channels"
	channelsMsgKey    = "Channels"
)

// AddChannelsDBToTransportSyncer adds channels db to transport syncer to the manager.
func AddChannelsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	createObjFunc := func() metav1.Object { return &channelv1.Channel{} }
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("channels-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncObjectsBundle(ctx, transportObj, channelsMsgKey, specDB, channelsTableName,
				createObjFunc, bundle.NewBaseObjectsBundle, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add channels db to transport syncer - %w", err)
	}

	return nil
}
