package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	specbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
)

const (
	// periodicApplyInterval = 5 * time.Second
	hohFieldManager = "mgh-agent"
)

// managedClusterLabelsBundleSyncer syncs managed clusters metadata from received bundles.
type managedClusterLabelsBundleSyncer struct {
	log                          logr.Logger
	latestBundle                 *specbundle.ManagedClusterLabelsSpecBundle
	managedClusterToTimestampMap map[string]*time.Time
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	latestBundleLock             sync.Mutex
}

func NewManagedClusterLabelSyncer(workers *workers.WorkerPool) *managedClusterLabelsBundleSyncer {
	return &managedClusterLabelsBundleSyncer{
		log:                          ctrl.Log.WithName("managed-clusters-labels-syncer"),
		latestBundle:                 nil,
		managedClusterToTimestampMap: make(map[string]*time.Time),
		workerPool:                   workers,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		latestBundleLock:             sync.Mutex{},
	}
}

func (syncer *managedClusterLabelsBundleSyncer) Sync(payload []byte) error {
	bundle := &specbundle.ManagedClusterLabelsSpecBundle{}
	if err := json.Unmarshal(payload, bundle); err != nil {
		return err
	}
	syncer.setLatestBundle(bundle) // uses latestBundle
	syncer.handleBundle()

	return nil
}

func (syncer *managedClusterLabelsBundleSyncer) setLatestBundle(newBundle *specbundle.ManagedClusterLabelsSpecBundle) {
	syncer.latestBundleLock.Lock()
	defer syncer.latestBundleLock.Unlock()

	syncer.latestBundle = newBundle
}

func (syncer *managedClusterLabelsBundleSyncer) handleBundle() {
	syncer.latestBundleLock.Lock()
	defer syncer.latestBundleLock.Unlock()

	for _, managedClusterLabelsSpec := range syncer.latestBundle.Objects {
		lastProcessedTimestampPtr := syncer.getManagedClusterLastProcessedTimestamp(managedClusterLabelsSpec.ClusterName)
		if managedClusterLabelsSpec.UpdateTimestamp.After(*lastProcessedTimestampPtr) { // handle (success) once
			syncer.bundleProcessingWaitingGroup.Add(1)
			syncer.updateManagedClusterAsync(managedClusterLabelsSpec, lastProcessedTimestampPtr)
		}
	}

	// ensure all updates and deletes have finished before reading next bundle
	syncer.bundleProcessingWaitingGroup.Wait()
}

func (syncer *managedClusterLabelsBundleSyncer) updateManagedClusterAsync(
	labelsSpec *specbundle.ManagedClusterLabelsSpec, lastProcessedTimestampPtr *time.Time,
) {
	syncer.workerPool.Submit(workers.NewJob(labelsSpec, func(ctx context.Context,
		k8sClient client.Client, obj interface{},
	) {
		defer syncer.bundleProcessingWaitingGroup.Done()

		syncer.log.V(2).Info("update the label bundle to ManagedCluster CR...")
		labelsSpec, ok := obj.(*specbundle.ManagedClusterLabelsSpec)
		if !ok {
			syncer.log.Error(errors.New("job obj is not a ManagedClusterLabelsSpec type"), "invald obj type")
		}

		managedCluster := &clusterv1.ManagedCluster{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Name: labelsSpec.ClusterName,
		}, managedCluster); k8serrors.IsNotFound(err) {
			syncer.log.Info("managed cluster ignored - not found", "name", labelsSpec.ClusterName)
			syncer.managedClusterMarkUpdated(labelsSpec, lastProcessedTimestampPtr) // if not found then irrelevant

			return
		} else if err != nil {
			syncer.log.Error(err, "failed to get managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		// enforce received labels state (overwrite if exists)
		for key, value := range labelsSpec.Labels {
			managedCluster.Labels[key] = value
		}

		// delete labels by key
		for _, labelKey := range labelsSpec.DeletedLabelKeys {
			delete(managedCluster.Labels, labelKey)
		}

		if err := syncer.updateManagedFieldEntry(managedCluster, labelsSpec); err != nil {
			syncer.log.Error(err, "failed to update managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		// update CR with replace API: fails if CR was modified since client.get
		if err := k8sClient.Update(ctx, managedCluster,
			&client.UpdateOptions{FieldManager: hohFieldManager}); err != nil {
			syncer.log.Error(err, "failed to update managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		syncer.log.V(2).Info("managed cluster updated", "name", labelsSpec.ClusterName)
		syncer.managedClusterMarkUpdated(labelsSpec, lastProcessedTimestampPtr)
	}))
}

func (syncer *managedClusterLabelsBundleSyncer) managedClusterMarkUpdated(
	labelsSpec *specbundle.ManagedClusterLabelsSpec, lastProcessedTimestampPtr *time.Time,
) {
	*lastProcessedTimestampPtr = labelsSpec.UpdateTimestamp
}

func (syncer *managedClusterLabelsBundleSyncer) getManagedClusterLastProcessedTimestamp(name string) *time.Time {
	timestamp, found := syncer.managedClusterToTimestampMap[name]
	if found {
		return timestamp
	}

	timestamp = &time.Time{}
	syncer.managedClusterToTimestampMap[name] = timestamp

	return timestamp
}

// updateManagedFieldEntry inserts/updates the hohFieldManager managed-field entry in a given managedCluster.
func (syncer *managedClusterLabelsBundleSyncer) updateManagedFieldEntry(managedCluster *clusterv1.ManagedCluster,
	managedClusterLabelsSpec *specbundle.ManagedClusterLabelsSpec,
) error {
	// create label fields
	labelFields := helper.LabelsField{Labels: map[string]struct{}{}}
	for key := range managedClusterLabelsSpec.Labels {
		labelFields.Labels[fmt.Sprintf("f:%s", key)] = struct{}{}
	}
	// create metadata field
	metadataField := helper.MetadataField{LabelsField: labelFields}

	metadataFieldRaw, err := json.Marshal(metadataField)
	if err != nil {
		return fmt.Errorf("failed to create ManagedFieldsEntry - %w", err)
	}
	// create entry
	managedFieldEntry := v1.ManagedFieldsEntry{
		Manager:    hohFieldManager,
		Operation:  v1.ManagedFieldsOperationUpdate,
		APIVersion: managedCluster.APIVersion,
		Time:       &v1.Time{Time: time.Now()},
		FieldsV1:   &v1.FieldsV1{Raw: metadataFieldRaw},
	}
	// get entry index
	index := -1

	for i, entry := range managedCluster.ManagedFields {
		if entry.Manager == hohFieldManager {
			index = i
			break
		}
	}
	// replace
	if index >= 0 {
		managedCluster.ManagedFields[index] = managedFieldEntry
	} else { // otherwise, insert
		managedCluster.ManagedFields = append(managedCluster.ManagedFields, managedFieldEntry)
	}

	return nil
}
