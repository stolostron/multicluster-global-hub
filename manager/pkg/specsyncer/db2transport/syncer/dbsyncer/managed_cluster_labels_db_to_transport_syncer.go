package dbsyncer

import (
	"context"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/transport"
	"github.com/stolostron/multicluster-globalhub/pkg/constants"
)

const managedClusterLabelsDBTableName = "managed_clusters_labels"

// AddManagedClusterLabelsDBToTransportSyncer adds managed-cluster labels db to transport syncer to the manager.
func AddManagedClusterLabelsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, transportObj transport.Transport,
	specSyncInterval time.Duration,
) error {
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("managed-cluster-labels-db-to-transport-syncer"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncManagedClusterLabelsBundles(ctx, transportObj,
				constants.ManagedClustersLabelsMsgKey, specDB,
				managedClusterLabelsDBTableName, lastSyncTimestampPtr)
		},
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster labels db to transport syncer - %w", err)
	}

	return nil
}

// syncManagedClusterLabelsBundles performs the actual sync logic and returns true if bundle was committed to transport,
// otherwise false.
func syncManagedClusterLabelsBundles(ctx context.Context, transportObj transport.Transport, transportBundleKey string,
	specDB db.SpecDB, dbTableName string, lastSyncTimestampPtr *time.Time,
) (bool, error) {
	lastUpdateTimestamp, err := specDB.GetLastUpdateTimestamp(ctx, dbTableName, false) // no resources in table
	if err != nil {
		return false, fmt.Errorf("unable to sync bundle - %w", err)
	}

	if !lastUpdateTimestamp.After(*lastSyncTimestampPtr) { // sync only if something has changed
		return false, nil
	}

	// if we got here, then the last update timestamp from db is after what we have in memory.
	// this means something has changed in db, syncing to transport.
	leafHubToLabelsSpecBundleMap,
		err := specDB.GetUpdatedManagedClusterLabelsBundles(ctx, dbTableName, lastSyncTimestampPtr)
	if err != nil {
		return false, fmt.Errorf("unable to sync bundle - %w", err)
	}

	// sync bundle per leaf hub
	for leafHubName, managedClusterLabelsBundle := range leafHubToLabelsSpecBundleMap {
		if err := syncToTransport(transportObj, leafHubName, transportBundleKey, lastUpdateTimestamp,
			managedClusterLabelsBundle); err != nil {
			return false, fmt.Errorf("unable to sync bundle to transport - %w", err)
		}
	}

	// updating value to retain same ptr between calls
	*lastSyncTimestampPtr = *lastUpdateTimestamp

	return true, nil
}
