package statuswatcher

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/db"
	"github.com/stolostron/multicluster-globalhub/manager/pkg/specsyncer/db2transport/intervalpolicy"
)

const (
	managedClusterLabelsSpecDBTableName   = "managed_clusters_labels"
	managedClusterLabelsStatusDBTableName = "managed_clusters"
	tempLeafHubNameFillInterval           = 10 * time.Second
)

// AddManagedClusterLabelsStatusWatcher adds managedClusterLabelsStatusWatcher to the manager.
func AddManagedClusterLabelsStatusWatcher(mgr ctrl.Manager, specDB db.SpecDB, statusDB db.StatusDB,
	deletedLabelsTrimmingInterval time.Duration,
) error {
	if err := mgr.Add(&managedClusterLabelsStatusWatcher{
		log:                   ctrl.Log.WithName("managed-cluster-labels-status-watcher"),
		specDB:                specDB,
		statusDB:              statusDB,
		labelsSpecTableName:   managedClusterLabelsSpecDBTableName,
		labelsStatusTableName: managedClusterLabelsStatusDBTableName,
		intervalPolicy:        intervalpolicy.NewExponentialBackoffPolicy(deletedLabelsTrimmingInterval),
	}); err != nil {
		return fmt.Errorf("failed to add managed-cluster labels status watcher - %w", err)
	}

	return nil
}

// managedClusterLabelsStatusWatcher watches the status managed-clusters status table to sync and update spec
// table where required (e.g., trim deleted_label_keys).
type managedClusterLabelsStatusWatcher struct {
	log                   logr.Logger
	specDB                db.SpecDB
	statusDB              db.StatusDB
	labelsSpecTableName   string
	labelsStatusTableName string
	intervalPolicy        intervalpolicy.IntervalPolicy
}

func (watcher *managedClusterLabelsStatusWatcher) Start(ctx context.Context) error {
	watcher.log.Info("initialized watcher", "spec table",
		fmt.Sprintf("spec.%s", watcher.labelsSpecTableName),
		"status table", fmt.Sprintf("status.%s", watcher.labelsStatusTableName))

	go watcher.updateDeletedLabelsPeriodically(ctx)

	<-ctx.Done() // blocking wait for cancel context event
	watcher.log.Info("stopped watcher", "spec table",
		fmt.Sprintf("spec.%s", watcher.labelsSpecTableName),
		"status table", fmt.Sprintf("status.%s", watcher.labelsStatusTableName))

	return nil
}

func (watcher *managedClusterLabelsStatusWatcher) updateDeletedLabelsPeriodically(ctx context.Context) {
	hubNameFillTicker := time.NewTicker(tempLeafHubNameFillInterval) // TODO: delete when nonk8s-api exposes name.
	labelsTrimmerTicker := time.NewTicker(watcher.intervalPolicy.GetInterval())

	for {
		select {
		case <-ctx.Done(): // we have received a signal to stop
			hubNameFillTicker.Stop()
			labelsTrimmerTicker.Stop()

			return

		case <-labelsTrimmerTicker.C:
			// define timeout of max execution interval on the update function
			ctxWithTimeout, cancelFunc := context.WithTimeout(ctx,
				watcher.intervalPolicy.GetMaxInterval())
			trimmed := watcher.trimDeletedLabelsByStatus(ctxWithTimeout)

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

		case <-hubNameFillTicker.C: // TODO: delete when nonk8s-api exposes name.
			// define timeout of max execution interval on the update function
			ctxWithTimeout, cancelFunc := context.WithTimeout(ctx,
				watcher.intervalPolicy.GetMaxInterval())
			watcher.fillMissingLeafHubNames(ctxWithTimeout)

			cancelFunc() // cancel child ctx and is used to cleanup resources once context expires or update is done.
		}
	}
}

func (watcher *managedClusterLabelsStatusWatcher) trimDeletedLabelsByStatus(ctx context.Context) bool {
	leafHubToLabelsSpecBundleMap, err := watcher.specDB.GetEntriesWithDeletedLabels(ctx, watcher.labelsSpecTableName)
	if err != nil {
		watcher.log.Error(err, "trimming cycle skipped")
		return false
	}

	result := true

	// iterate over entries
	for _, managedClusterLabelsSpecBundle := range leafHubToLabelsSpecBundleMap {
		// fetch actual labels status reflected in status DB
		for _, managedClusterLabelsSpec := range managedClusterLabelsSpecBundle.Objects {
			labelsStatus, err := watcher.statusDB.GetManagedClusterLabelsStatus(ctx, watcher.labelsStatusTableName,
				managedClusterLabelsSpecBundle.LeafHubName, managedClusterLabelsSpec.ClusterName)
			if err != nil {
				watcher.log.Error(err, "skipped trimming managed cluster labels spec",
					"leafHub", managedClusterLabelsSpecBundle.LeafHubName,
					"managedCluster", managedClusterLabelsSpec.ClusterName,
					"version", managedClusterLabelsSpec.Version)

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
			if len(deletedLabelKeysStillInStatus) ==
				len(managedClusterLabelsSpec.DeletedLabelKeys) {
				continue
			}

			if err := watcher.specDB.UpdateDeletedLabelKeys(ctx, watcher.labelsSpecTableName,
				managedClusterLabelsSpec.Version, managedClusterLabelsSpecBundle.LeafHubName,
				managedClusterLabelsSpec.ClusterName, deletedLabelKeysStillInStatus); err != nil {
				watcher.log.Error(err, "failed to trim deleted_label_keys",
					"leafHub", managedClusterLabelsSpecBundle.LeafHubName,
					"managedCluster", managedClusterLabelsSpec.ClusterName,
					"version", managedClusterLabelsSpec.Version)

				result = false

				continue
			}

			watcher.log.Info("trimmed labels successfully", "leafHub",
				managedClusterLabelsSpecBundle.LeafHubName,
				"managedCluster", managedClusterLabelsSpec.ClusterName, "version", managedClusterLabelsSpec.Version)
		}
	}

	return result
}

// TODO: once non-k8s-restapi exposes hub names, remove line.
func (watcher *managedClusterLabelsStatusWatcher) fillMissingLeafHubNames(ctx context.Context) {
	entries, err := watcher.specDB.GetEntriesWithoutLeafHubName(ctx, watcher.labelsSpecTableName)
	if err != nil {
		watcher.log.Error(err, "failed to fetch entries with no leaf-hub-name from spec db table")
		return
	}

	// update leaf hub name for each entry
	for _, managedClusterLabelsSpec := range entries {
		leafHubName, err := watcher.statusDB.GetManagedClusterLeafHubName(ctx,
			watcher.labelsStatusTableName, managedClusterLabelsSpec.ClusterName)
		if err != nil {
			watcher.log.Error(err, "failed to get leaf-hub name from status db table",
				"managedCluster", managedClusterLabelsSpec.ClusterName,
				"version", managedClusterLabelsSpec.Version)

			continue
		}

		// update leaf hub name
		if err := watcher.specDB.UpdateLeafHubName(ctx, watcher.labelsSpecTableName,
			managedClusterLabelsSpec.Version, managedClusterLabelsSpec.ClusterName, leafHubName); err != nil {
			watcher.log.Error(err, "failed to update leaf hub name for managed cluster in spec db table",
				"managedCluster", managedClusterLabelsSpec.ClusterName,
				"version", managedClusterLabelsSpec.Version, "leafHub", leafHubName)

			continue
		}

		watcher.log.Info("updated leaf hub name for managed cluster in spec db table",
			"managedCluster", managedClusterLabelsSpec.ClusterName,
			"leafHub", leafHubName, "version", managedClusterLabelsSpec.Version)
	}
}
