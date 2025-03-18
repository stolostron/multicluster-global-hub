package syncers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const managedClusterLabelsDBTableName = "managed_clusters_labels"

// AddManagedClusterLabelsDBToTransportSyncer adds managed-cluster labels db to transport syncer to the manager.
func AddManagedClusterLabelsDBToTransportSyncer(mgr ctrl.Manager, specDB specdb.SpecDB, producer transport.Producer,
	specSyncInterval time.Duration,
) error {
	lastSyncTimestampPtr := &time.Time{}

	if err := mgr.Add(&genericDBToTransportSyncer{
		log:            logger.ZapLogger("db-to-transport-syncer-managedclusterlabel"),
		intervalPolicy: interval.NewExponentialBackoffPolicy(specSyncInterval),
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
	specDB specdb.SpecDB, dbTableName string, lastSyncTimestampPtr *time.Time,
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
	leafHubToLabelsSpecBundleMap, err := getUpdatedManagedClusterLabelsBundles(lastSyncTimestampPtr)
	if err != nil {
		return false, fmt.Errorf("unable to sync bundle - %w", err)
	}

	// sync bundle per leaf hub
	for leafHubName, managedClusterLabelsBundle := range leafHubToLabelsSpecBundleMap {
		payloadBytes, err := json.Marshal(managedClusterLabelsBundle)
		if err != nil {
			return false, fmt.Errorf("failed to sync marshal bundle(%s)", transportBundleKey)
		}

		evt := utils.ToCloudEvent(transportBundleKey, constants.CloudEventGlobalHubClusterName, leafHubName, payloadBytes)
		if err := producer.SendEvent(ctx, evt); err != nil {
			return false, fmt.Errorf("failed to sync message(%s) from table(%s) to destination(%s) - %w",
				leafHubName, dbTableName, transport.Broadcast, err)
		}
	}

	// updating value to retain same ptr between calls
	*lastSyncTimestampPtr = *lastUpdateTimestamp

	return true, nil
}

// getUpdatedManagedClusterLabelsBundles returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects
// belonging to a leaf-hub that had at least once update since the given timestamp, from a specific table.
func getUpdatedManagedClusterLabelsBundles(timestamp *time.Time,
) (map[string]*spec.ManagedClusterLabelsSpecBundle, error) {
	db := database.GetGorm()
	// select ManagedClusterLabelsSpec entries information from DB
	rows, err := db.Raw(fmt.Sprintf(`SELECT * FROM spec.%[1]s WHERE leaf_hub_name IN (SELECT DISTINCT(leaf_hub_name) 
		from spec.%[1]s WHERE updated_at::timestamp > timestamp '%[2]s') AND leaf_hub_name <> ''`,
		managedClusterLabelsDBTableName, timestamp.Format(time.RFC3339Nano))).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return getManagedClusterLabelBundleByRows(db, rows)
}

func getManagedClusterLabelBundleByRows(db *gorm.DB, rows *sql.Rows) (
	map[string]*spec.ManagedClusterLabelsSpecBundle, error,
) {
	leafHubToLabelsSpecBundleMap := make(map[string]*spec.ManagedClusterLabelsSpecBundle)
	for rows.Next() {
		managedClusterLabel := models.ManagedClusterLabel{}
		if err := db.ScanRows(rows, &managedClusterLabel); err != nil {
			return nil, fmt.Errorf("error reading managed cluster label from table - %w", err)
		}

		// create ManagedClusterLabelsSpecBundle if not mapped for leafHub
		managedClusterLabelsSpecBundle, found := leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName]
		if !found {
			managedClusterLabelsSpecBundle = &spec.ManagedClusterLabelsSpecBundle{
				Objects:     []*spec.ManagedClusterLabelsSpec{},
				LeafHubName: managedClusterLabel.LeafHubName,
			}
			leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName] = managedClusterLabelsSpecBundle
		}

		labels := map[string]string{}
		err := json.Unmarshal(managedClusterLabel.Labels, &labels)
		if err != nil {
			return nil, fmt.Errorf("error to unmarshal labels - %w", err)
		}

		deletedKeys := []string{}
		err = json.Unmarshal(managedClusterLabel.DeletedLabelKeys, &deletedKeys)
		if err != nil {
			return nil, fmt.Errorf("error to unmarshal deletedKeys - %w", err)
		}

		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects,
			&spec.ManagedClusterLabelsSpec{
				ClusterName:      managedClusterLabel.ManagedClusterName,
				Version:          int64(managedClusterLabel.Version),
				UpdateTimestamp:  managedClusterLabel.UpdatedAt,
				Labels:           labels,
				DeletedLabelKeys: deletedKeys,
			})
	}

	return leafHubToLabelsSpecBundleMap, nil
}
