package syncers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/workers"
	specbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/spec"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	// periodicApplyInterval = 5 * time.Second
	hohFieldManager = "mgh-agent"
)

// managedClusterLabelsBundleSyncer syncs managed clusters metadata from received bundles.
type managedClusterLabelsBundleSyncer struct {
	log                          *zap.SugaredLogger
	latestBundle                 *specbundle.ManagedClusterLabelsSpecBundle
	managedClusterToTimestampMap map[string]*time.Time
	workerPool                   *workers.WorkerPool
	bundleProcessingWaitingGroup sync.WaitGroup
	latestBundleLock             sync.Mutex
}

func NewManagedClusterLabelSyncer(workers *workers.WorkerPool) *managedClusterLabelsBundleSyncer {
	return &managedClusterLabelsBundleSyncer{
		log:                          logger.ZapLogger("managed-clusters-labels-syncer"),
		latestBundle:                 nil,
		managedClusterToTimestampMap: make(map[string]*time.Time),
		workerPool:                   workers,
		bundleProcessingWaitingGroup: sync.WaitGroup{},
		latestBundleLock:             sync.Mutex{},
	}
}

func (syncer *managedClusterLabelsBundleSyncer) Sync(ctx context.Context, evt *cloudevents.Event) error {
	payload := evt.Data()
	bundle := &specbundle.ManagedClusterLabelsSpecBundle{}
	if err := json.Unmarshal(payload, bundle); err != nil {
		return err
	}
	syncer.log.Debugf("bundle: %v", bundle)
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
		syncer.log.Debugf("managedClusterLabelsSpec: %v", managedClusterLabelsSpec)

		lastProcessedTimestampPtr := syncer.getManagedClusterLastProcessedTimestamp(managedClusterLabelsSpec.ClusterName)
		if managedClusterLabelsSpec.UpdateTimestamp.After(*lastProcessedTimestampPtr) { // handle (success) once
			syncer.bundleProcessingWaitingGroup.Add(1)
			syncer.updateManagedClusterAsync(managedClusterLabelsSpec, lastProcessedTimestampPtr)
		}
	}

	// ensure all updates and deletes have finished before reading next bundle
	syncer.bundleProcessingWaitingGroup.Wait()
}

func (s *managedClusterLabelsBundleSyncer) updateManagedClusterAsync(
	labelsSpec *specbundle.ManagedClusterLabelsSpec, lastProcessedTimestampPtr *time.Time,
) {
	s.workerPool.Submit(workers.NewJob(labelsSpec, func(ctx context.Context,
		k8sClient client.Client, obj interface{},
	) {
		defer s.bundleProcessingWaitingGroup.Done()

		s.log.Debug("update the label bundle to ManagedCluster CR...")
		labelsSpec, ok := obj.(*specbundle.ManagedClusterLabelsSpec)
		if !ok {
			s.log.Error(errors.New("job obj is not a ManagedClusterLabelsSpec type"), "invalid obj type")
		}

		managedCluster := &clusterv1.ManagedCluster{}
		if err := k8sClient.Get(ctx, client.ObjectKey{
			Name: labelsSpec.ClusterName,
		}, managedCluster); k8serrors.IsNotFound(err) {
			s.log.Info("managed cluster ignored - not found", "name", labelsSpec.ClusterName)
			s.managedClusterMarkUpdated(labelsSpec, lastProcessedTimestampPtr) // if not found then irrelevant

			return
		} else if err != nil {
			s.log.Error(err, "failed to get managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		if managedCluster.Labels == nil {
			managedCluster.Labels = make(map[string]string)
		}

		// enforce received labels state (overwrite if exists)
		for key, value := range labelsSpec.Labels {
			managedCluster.Labels[key] = value
		}

		// delete labels by key
		for _, labelKey := range labelsSpec.DeletedLabelKeys {
			delete(managedCluster.Labels, labelKey)
		}

		if err := s.updateManagedFieldEntry(managedCluster, labelsSpec); err != nil {
			s.log.Error(err, "failed to update managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		// update CR with replace API: fails if CR was modified since client.get
		if err := k8sClient.Update(ctx, managedCluster, &client.UpdateOptions{FieldManager: hohFieldManager}); err != nil {
			s.log.Error(err, "failed to update managed cluster", "name", labelsSpec.ClusterName)
			return
		}

		s.log.Debug("managed cluster updated ", " name ", labelsSpec.ClusterName)
		s.managedClusterMarkUpdated(labelsSpec, lastProcessedTimestampPtr)
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
	labelFields := utils.LabelsField{Labels: map[string]struct{}{}}
	for key := range managedClusterLabelsSpec.Labels {
		labelFields.Labels[fmt.Sprintf("f:%s", key)] = struct{}{}
	}
	// create metadata field
	metadataField := utils.MetadataField{LabelsField: labelFields}

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
