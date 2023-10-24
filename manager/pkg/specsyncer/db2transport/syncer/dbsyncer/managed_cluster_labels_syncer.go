package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/db/gorm"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/specsyncer/db2transport/intervalpolicy"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	labelsTableName             = "managed_clusters_labels"
	clusterTableName            = "managed_clusters"
	tempLeafHubNameFillInterval = 10 * time.Second
)

// AddManagedClusterLabelsSyncer adds managedClusterLabelsStatusWatcher to the manager.
func AddManagedClusterLabelsSyncer(mgr ctrl.Manager, deletedLabelsTrimmingInterval time.Duration) error {
	if err := mgr.Add(&managedClusterLabelsStatusWatcher{
		log:            ctrl.Log.WithName("managed-cluster-labels-syncer"),
		specDB:         gorm.NewGormSpecDB(),
		intervalPolicy: intervalpolicy.NewExponentialBackoffPolicy(deletedLabelsTrimmingInterval),
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster labels status watcher - %w", err)
	}

	return nil
}

// managedClusterLabelsStatusWatcher watches the status managed cluster table to update labels
// table where required (e.g., trim deleted_label_keys).
type managedClusterLabelsStatusWatcher struct {
	log            logr.Logger
	specDB         db.SpecDB
	intervalPolicy intervalpolicy.IntervalPolicy
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

			ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, watcher.intervalPolicy.GetMaxInterval())

			// update the deleted label keys to label table by the managed cluster table
			trimmed := watcher.updateDeletedLabelsByManagedCluster(ctxWithTimeout)

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
			ctxWithTimeout, cancelFunc := context.WithTimeout(ctx,
				watcher.intervalPolicy.GetMaxInterval())
			watcher.fillMissingLeafHubNames(ctxWithTimeout)

			cancelFunc() // cancel child ctx and is used to cleanup resources once context expires or update is done.
		}
	}
}

func (watcher *managedClusterLabelsStatusWatcher) updateDeletedLabelsByManagedCluster(ctx context.Context) bool {
	leafHubToLabelsSpecBundleMap, err := getManagedClusterLabelsWithDeletedKey(ctx)
	if err != nil {
		fmt.Println("====================1", err.Error())
		watcher.log.Error(err, "trimming cycle skipped")
		return false
	}
	fmt.Println("====================2", err.Error())
	result := true
	// iterate over entries
	for _, managedClusterLabelsSpecBundle := range leafHubToLabelsSpecBundleMap {
		// fetch actual labels status reflected in status DB
		for _, managedClusterLabelsSpec := range managedClusterLabelsSpecBundle.Objects {
			labelsStatus, err := getLabelsFromManagedCluster(ctx, managedClusterLabelsSpecBundle.LeafHubName,
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

			err = updateDeletedKeysToLabelTable(ctx,
				managedClusterLabelsSpec.Version, managedClusterLabelsSpecBundle.LeafHubName,
				managedClusterLabelsSpec.ClusterName, deletedLabelKeysStillInStatus)
			if err != nil {
				watcher.log.Error(err, "failed to sync deleted_label_keys to label tables")
				result = false
				continue
			}

			watcher.log.V(2).Info("update label table by managed cluster table successfully", "leafHub",
				managedClusterLabelsSpecBundle.LeafHubName,
				"managedCluster", managedClusterLabelsSpec.ClusterName, "version", managedClusterLabelsSpec.Version)
		}
	}

	return result
}

func (watcher *managedClusterLabelsStatusWatcher) fillMissingLeafHubNames(ctx context.Context) {
	entities, err := getLabelsWithoutLeafHubName(ctx)
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
func getManagedClusterLabelsWithDeletedKey(ctx context.Context) (
	map[string]*spec.ManagedClusterLabelsSpecBundle, error,
) {
	db := database.GetGorm()
	var managedClusterLabels []models.ManagedClusterLabel
	deleted_key := []string{}
	payload, err := json.Marshal(deleted_key)
	if err != nil {
		return nil, err
	}
	err = db.Where("deleted_label_keys <> ? AND leaf_hub_name <> ?", payload, "").Find(&managedClusterLabels).Error
	if err != nil {
		return nil, err
	}

	leafHubToLabelsSpecBundleMap := make(map[string]*spec.ManagedClusterLabelsSpecBundle)

	for _, managedClusterLabel := range managedClusterLabels {
		labels := map[string]string{}
		fmt.Println("managedClusterLabel", managedClusterLabel)
		if err := json.Unmarshal(managedClusterLabel.Labels, labels); err != nil {
			return nil, err
		}
		deletedKeys := []string{}
		if err := json.Unmarshal(managedClusterLabel.DeletedLabelKeys, deletedKeys); err != nil {
			return nil, err
		}
		managedClusterLabelsSpecBundle, found := leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName]
		if !found {
			managedClusterLabelsSpecBundle = &spec.ManagedClusterLabelsSpecBundle{
				Objects:     []*spec.ManagedClusterLabelsSpec{},
				LeafHubName: managedClusterLabel.LeafHubName,
			}
			leafHubToLabelsSpecBundleMap[managedClusterLabel.LeafHubName] = managedClusterLabelsSpecBundle
		}
		// append entry to bundle
		managedClusterLabelsSpecBundle.Objects = append(managedClusterLabelsSpecBundle.Objects,
			&spec.ManagedClusterLabelsSpec{
				ClusterName:      managedClusterLabel.ManagedClusterName,
				DeletedLabelKeys: deletedKeys,
				Labels:           labels,
				Version:          int64(managedClusterLabel.Version),
				UpdateTimestamp:  managedClusterLabel.UpdatedAt,
			})

	}
	return leafHubToLabelsSpecBundleMap, nil
}

// Return the labels present in managed-cluster CR metadata from a specific table.
func getLabelsFromManagedCluster(ctx context.Context, leafHubName string, managedClusterName string,
) (map[string]string, error) {
	db := database.GetGorm()

	var labelsPayload []byte
	if err := db.Raw(fmt.Sprintf(`SELECT payload->'metadata'->'labels' FROM status.%s WHERE 
		leaf_hub_name=? AND payload->'metadata'->>'name'=?`, clusterTableName), leafHubName,
		managedClusterName).Row().Scan(&labelsPayload); err != nil {
		return nil, fmt.Errorf("error reading from table status.%s - %w", clusterTableName, err)
	}

	labels := make(map[string]string)
	err := json.Unmarshal(labelsPayload, &labels)
	if err != nil {
		return nil, err
	}
	return labels, nil
}

// UpdateDeletedLabelKeys updates deleted_label_keys value for a managed cluster entry under
// optimistic concurrency approach.
func updateDeletedKeysToLabelTable(ctx context.Context, readVersion int64,
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
func getLabelsWithoutLeafHubName(ctx context.Context) ([]*spec.ManagedClusterLabelsSpec, error) {
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
	db := database.GetGorm()
	var leafHubName string
	if err := db.Raw(fmt.Sprintf(`SELECT leaf_hub_name FROM status.%s WHERE 
		payload->'metadata'->>'name' = ?`, clusterTableName), managedClusterName).Row().Scan(&leafHubName); err != nil {
		return "", fmt.Errorf("error reading from table status.%s - %w", clusterTableName, err)
	}
	return leafHubName, nil
}

// updateLeafHubNameByClusterName updates leaf hub name for a given managed cluster under optimistic concurrency.
func updateLeafHubNameByClusterName(readVersion int64, managedClusterName string, leafHubName string) error {
	db := database.GetGorm()
	if result := db.Exec(fmt.Sprintf(`UPDATE spec.%s SET updated_at=now(),leaf_hub_name=?,version=? 
		WHERE managed_cluster_name=? AND version=?`, labelsTableName), leafHubName, readVersion+1,
		managedClusterName, readVersion); result.Error != nil {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s - %w", labelsTableName, result.Error)
	} else if result.RowsAffected == 0 {
		return fmt.Errorf("failed to update managed cluster labels row in spec.%s", labelsTableName)
	}
	return nil
}
