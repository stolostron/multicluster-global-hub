package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const managedClusterLabelsDBTableName = "managed_clusters_labels"

// AddManagedClusterLabelsDBToTransportSyncer adds managed-cluster labels db to transport syncer to the manager.
func AddManagedClusterLabelsDBToTransportSyncer(mgr ctrl.Manager, specDB db.SpecDB, producer transport.Producer,
	specSyncInterval time.Duration,
) error {
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            ctrl.Log.WithName("db-to-transport-syncer-managedclusterlabel"),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(specSyncInterval),
		syncBundleFunc: func(ctx context.Context) (bool, error) {
			return syncManagedClusterLabelsBundles(ctx, producer,
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
func syncManagedClusterLabelsBundles(ctx context.Context, producer transport.Producer, transportBundleKey string,
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
		payloadBytes, err := json.Marshal(managedClusterLabelsBundle)
		if err != nil {
			return false, fmt.Errorf("failed to sync marshal bundle(%s)", transportBundleKey)
		}
		if err := producer.Send(ctx, &transport.Message{
			Destination: leafHubName,
			ID:          transportBundleKey,
			MsgType:     constants.SpecBundle,
			Version:     lastUpdateTimestamp.Format(timeFormat),
			Payload:     payloadBytes,
		}); err != nil {
			return false, fmt.Errorf("failed to sync message(%s) from table(%s) to destination(%s) - %w",
				transportBundleKey, dbTableName, transport.Broadcast, err)
		}
	}

	// updating value to retain same ptr between calls
	*lastSyncTimestampPtr = *lastUpdateTimestamp

	return true, nil
}
