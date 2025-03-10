package syncers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var log = logger.ZapLogger("labels-watcher")

const (
	labelsTableName             = "managed_clusters_labels"
	clusterTableName            = "managed_clusters"
	tempLeafHubNameFillInterval = 10 * time.Second
)

// AddManagedClusterLabelsSyncer adds managedClusterLabelsStatusWatcher to the manager.
func AddManagedClusterLabelsSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := mgr.Add(&managedClusterLabelsStatusWatcher{
		log:            logger.ZapLogger("managed-cluster-labels-syncer"),
		specDB:         gorm.NewGormSpecDB(),
		intervalPolicy: interval.NewExponentialBackoffPolicy(deletedLabelsTrimmingInterval),
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster labels status watcher - %w", err)
	}

	return nil
}

// managedClusterLabelsStatusWatcher watches the status managed cluster table to update labels
// table where required (e.g., trim deleted_label_keys).
type managedClusterLabelsStatusWatcher struct {
	log            *zap.SugaredLogger
	specDB         specdb.SpecDB
	intervalPolicy interval.IntervalPolicy
}

func (watcher *managedClusterLabelsStatusWatcher) Start(ctx context.Context) error {
	watcher.log.Info("initialized watcher", "spec", labelsTableName, "status table", clusterTableName)
	go watcher.updateDeletedLabelsPeriodically(ctx)
	<-ctx.Done() // blocking wait for cancel context event
	watcher.log.Info("stopped watcher", "spec", labelsTableName, "status table", clusterTableName)
	return nil
}

func (watcher *managedClusterLabelsStatusWatcher) updateDeletedLabelsPeriodically(ctx context.Context) {
	hubNameFillTicker := time.NewTicker(tempLeafHubNameFillInterval)
	labelsTrimmerTicker := time.NewTicker(watcher.intervalPolicy.GetInterval())

	for {
		select {
		case <-ctx.Done(): // we have received a signal to stop
			hubNameFillTicker.Stop()
			labelsTrimmerTicker.Stop()

			return

		case <-labelsTrimmerTicker.C:
			log.Debugf("labelsTrimmerTicker")
			_, cancelFunc := context.WithTimeout(ctx, watcher.intervalPolicy.GetMaxInterval())

			// update the deleted label keys to label table by the managed cluster table
			trimmed := watcher.updateDeletedLabelsByManagedCluster()

			cancelFunc() // cancel child ctx and is used to cleanup resources once context expires or update is done.

			// get current trimming interval
			currentInterval := watcher.intervalPolicy.GetInterval()

			// notify policy whether trimming was actually performed or skipped
			if trimmed {
				watcher.intervalPolicy.Evaluate()
			} else {
				watcher.intervalPolicy.Reset()
			}

			// get reevaluated trimming interval
			reevaluatedInterval := watcher.intervalPolicy.GetInterval()

			// reset ticker if needed
			if currentInterval != reevaluatedInterval {
				labelsTrimmerTicker.Reset(reevaluatedInterval)
				watcher.log.Info(fmt.Sprintf("trimming interval has been reset to %s", reevaluatedInterval.String()))
			}

		case <-hubNameFillTicker.C:
			// define timeout of max execution interval on the update function
			_, cancelFunc := context.WithTimeout(ctx,
				watcher.intervalPolicy.GetMaxInterval())
			watcher.fillMissingLeafHubNames()

			cancelFunc() // cancel child ctx and is used to cleanup resources once context expires or update is done.
		}
	}
}

func (watcher *managedClusterLabelsStatusWatcher) updateDeletedLabelsByManagedCluster() bool {
	leafHubToLabelsSpecBundleMap, err := getLabelBundleWithDeletedKey()
	log.Debugf("leafHubToLabelsSpecBundleMap: %v, err: %v", leafHubToLabelsSpecBundleMap, err)
	if err != nil {
		watcher.log.Error(err, "trimming cycle skipped")
		return false
	}
	log.Debugf("updateDeletedLabelsByManagedCluster")
	conn := database.GetConn()
	err = database.Lock(conn)
	if err != nil {
		watcher.log.Error(err, "failed to lock db")
		return false
	}
	defer database.Unlock(conn)

	result := true
	// iterate over entries
	log.Debugf("updateDeletedLabelsByManagedCluster")

	for _, managedClusterLabelsSpecBundle := range leafHubToLabelsSpecBundleMap {
		// fetch actual labels status reflected in status DB
		for _, managedClusterLabelsSpec := range managedClusterLabelsSpecBundle.Objects {
			log.Debugf("managedClusterLabelsSpec: %v", managedClusterLabelsSpec)
			labelsStatus, err := getLabelsFromManagedCluster(managedClusterLabelsSpecBundle.LeafHubName,
				managedClusterLabelsSpec.ClusterName)
			if err != nil {
				watcher.log.Error(err, "failed to get the label from managed cluster")
				result = false
				continue
			}

			// check which deleted label keys still appear in status
			deletedLabelKeysStillInStatus := make([]string, 0)

			for _, key := range managedClusterLabelsSpec.DeletedLabelKeys {
				if _, found := labelsStatus[key]; found {
					deletedLabelKeysStillInStatus = append(deletedLabelKeysStillInStatus, key)
				}
			}

			// if deleted labels did not change then skip
			if len(deletedLabelKeysStillInStatus) == len(managedClusterLabelsSpec.DeletedLabelKeys) {
				continue
			}

			err = updateDeletedKeysToLabelTable(
				managedClusterLabelsSpec.Version, managedClusterLabelsSpecBundle.LeafHubName,
				managedClusterLabelsSpec.ClusterName, deletedLabelKeysStillInStatus)
			if err != nil {
				watcher.log.Error(err, "failed to sync deleted_label_keys to label tables")
				result = false
				continue
			}
			log.Debugf("update label table by managed cluster table successfully", "leafHub",
				managedClusterLabelsSpecBundle.LeafHubName,
				"managedCluster", managedClusterLabelsSpec.ClusterName, "version", managedClusterLabelsSpec.Version)

			watcher.log.Debugw("update label table by managed cluster table successfully", "leafHub",
				managedClusterLabelsSpecBundle.LeafHubName,
				"managedCluster", managedClusterLabelsSpec.ClusterName, "version", managedClusterLabelsSpec.Version)
		}
	}
	log.Debugf("result: %v", result)
	return result
}

func (watcher *managedClusterLabelsStatusWatcher) fillMissingLeafHubNames() {
	entities, err := getLabelsWithoutLeafHubName()
	if err != nil {
		watcher.log.Error(err, "failed to fetch entries with no leaf-hub-name from spec db table")
		return
	}

	// update leaf hub name for each entity
	for _, managedClusterLabelsSpec := range entities {
		leafHubName, err := getLeafHubNameByManagedCluster(managedClusterLabelsSpec.ClusterName)
		if err != nil {
			watcher.log.Error(err, "failed to get leaf-hub name from status db table")
			continue
		}

		// update leaf hub name by cluster name and version
		if err := updateLeafHubNameByClusterName(managedClusterLabelsSpec.Version,
			managedClusterLabelsSpec.ClusterName, leafHubName); err != nil {
			watcher.log.Error(err, "failed to update leaf hub name for managed cluster in spec db table")
			continue
		}

		watcher.log.Info("updated leaf hub name for managed cluster in spec db table")
	}
}

// returns a map of leaf-hub -> ManagedClusterLabelsSpecBundle of objects that have a
// none-empty deleted-label-keys column.
func getLabelBundleWithDeletedKey() (
	map[string]*spec.ManagedClusterLabelsSpecBundle, error,
) {
	db := database.GetGorm()
	rows, err := db.Raw(`SELECT * FROM spec.managed_clusters_labels WHERE deleted_label_keys <> ? AND leaf_hub_name <> ?`,
		[]byte("[]"), "").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return getManagedClusterLabelBundleByRows(db, rows)
}

// Return the labels present in managed-cluster CR metadata from a specific table.
func getLabelsFromManagedCluster(leafHubName string, managedClusterName string,
) (map[string]string, error) {
	db := database.GetGorm()

	var labelsPayload []byte
	err := db.Raw(fmt.Sprintf(`SELECT payload->'metadata'->'labels' FROM status.%s WHERE 
		leaf_hub_name=? AND payload->'metadata'->>'name'=?`, clusterTableName), leafHubName,
		managedClusterName).Row().Scan(&labelsPayload)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("error reading from table status.%s - %w", clusterTableName, err)
	}

	labels := make(map[string]string)
	if err == nil {
		if e := json.Unmarshal(labelsPayload, &labels); e != nil {
			return nil, e
		}
	}
	return labels, nil
}

// UpdateDeletedLabelKeys updates deleted_label_keys value for a managed cluster entry under
// optimistic concurrency approach.
func updateDeletedKeysToLabelTable(readVersion int64,
	leafHubName string, managedClusterName string, deletedLabelKeys []string,
) error {
	deletedLabelsJSON, err := json.Marshal(deletedLabelKeys)
	if err != nil {
		return fmt.Errorf("failed to marshal deleted labels - %w", err)
	}
	db := database.GetGorm()
	if result := db.Exec(fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),deleted_label_keys=?,
		version=? WHERE leaf_hub_name=? AND managed_cluster_name=? AND version=?`, labelsTableName), deletedLabelsJSON,
		readVersion+1, leafHubName, managedClusterName, readVersion); result.Error != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", labelsTableName, err)
	} else if result.RowsAffected == 0 {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s", labelsTableName)
	}
	return nil
}

// getLabelsWithoutLeafHubName returns a slice of ManagedClusterLabelsSpec that are missing leaf hub name.
func getLabelsWithoutLeafHubName() ([]*spec.ManagedClusterLabelsSpec, error) {
	db := database.GetGorm()
	rows, err := db.Raw(fmt.Sprintf(`SELECT managed_cluster_name, version FROM spec.%s WHERE 
			leaf_hub_name = ''`, labelsTableName)).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to read from spec.%s - %w", labelsTableName, err)
	}
	defer rows.Close()

	managedClusterLabelsSpecSlice := make([]*spec.ManagedClusterLabelsSpec, 0)

	for rows.Next() {
		var (
			managedClusterName string
			version            int64
		)
		if err := rows.Scan(&managedClusterName, &version); err != nil {
			return nil, fmt.Errorf("error reading from spec.%s - %w", labelsTableName, err)
		}
		managedClusterLabelsSpecSlice = append(managedClusterLabelsSpecSlice, &spec.ManagedClusterLabelsSpec{
			ClusterName: managedClusterName,
			Version:     version,
		})
	}

	return managedClusterLabelsSpecSlice, nil
}

// getLeafHubNameByManagedCluster returns leaf-hub name for a given managed cluster from a specific table.
func getLeafHubNameByManagedCluster(managedClusterName string) (string, error) {
	var leafHubName string

	db := database.GetGorm()

	if err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name FROM status.%s WHERE 
		payload->'metadata'->>'name' = ?`, clusterTableName), managedClusterName).Row().Scan(&leafHubName); err != nil {
		return "", fmt.Errorf("error reading from table status.%s - %w", clusterTableName, err)
	}
	return leafHubName, nil
}

// updateLeafHubNameByClusterName updates leaf hub name for a given managed cluster under optimistic concurrency.
func updateLeafHubNameByClusterName(readVersion int64, managedClusterName string, leafHubName string) error {
	db := database.GetGorm()
	conn := database.GetConn()

	err := database.Lock(conn)
	if err != nil {
		return err
	}
	defer database.Unlock(conn)
	if result := db.Exec(fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),leaf_hub_name=?,version=? 
		WHERE managed_cluster_name=? AND version=?`, labelsTableName), leafHubName, readVersion+1,
		managedClusterName, readVersion); result.Error != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", labelsTableName, result.Error)
	} else if result.RowsAffected == 0 {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s", labelsTableName)
	}
	return nil
}
