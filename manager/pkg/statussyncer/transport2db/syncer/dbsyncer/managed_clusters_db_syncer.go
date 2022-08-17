package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/helpers"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewManagedClustersDBSyncer creates a new instance of ManagedClustersDBSyncer.
func NewManagedClustersDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &ManagedClustersDBSyncer{
		log:              log,
		createBundleFunc: bundle.NewManagedClustersStatusBundle,
	}

	log.Info("initialized managed clusters db syncer")

	return dbSyncer
}

// ManagedClustersDBSyncer implements managed clusters db sync business logic.
type ManagedClustersDBSyncer struct {
	log              logr.Logger
	createBundleFunc bundle.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *ManagedClustersDBSyncer) RegisterCreateBundleFunctions(transportInstance transport.Transport) {
	transportInstance.Register(&transport.BundleRegistration{
		MsgID:            constants.ManagedClustersMsgKey,
		CreateBundleFunc: syncer.createBundleFunc,
		Predicate:        func() bool { return true }, // always get managed clusters bundles
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler function need to do "diff" between objects received in the bundle and the objects in db.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the db.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *ManagedClustersDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.ManagedClustersPriority,
		status.CompleteStateMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle bundle.Bundle, dbClient db.StatusTransportBridgeDB) error {
			return syncer.handleManagedClustersBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *ManagedClustersDBSyncer) handleManagedClustersBundle(ctx context.Context, bundle bundle.Bundle,
	dbClient db.ManagedClustersStatusDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	clustersFromDB, err := dbClient.GetManagedClustersByLeafHub(ctx, db.StatusSchema, db.ManagedClustersTableName,
		leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub managed clusters from db - %w", err)
	}
	// batch is per leaf hub, therefore no need to specify leafHubName in Insert/Update/Delete
	batchBuilder := dbClient.NewManagedClustersBatchBuilder(db.StatusSchema,
		db.ManagedClustersTableName, leafHubName)

	for _, object := range bundle.GetObjects() {
		cluster, ok := object.(*clusterv1.ManagedCluster)
		if !ok {
			continue // do not handle objects other than ManagedCluster
		}

		resourceVersionFromDB, clusterExistsInDB := clustersFromDB[cluster.GetName()]
		if !clusterExistsInDB { // cluster not found in the db table
			batchBuilder.Insert(cluster, db.ErrorNone)
			continue
		}

		delete(clustersFromDB, cluster.GetName()) // if we got here, cluster exists both in db and in received bundle.

		if cluster.GetResourceVersion() == resourceVersionFromDB {
			continue // update cluster in db only if what we got is a different (newer) version of the resource
		}

		batchBuilder.Update(cluster.GetName(), cluster)
	}
	// delete clusters that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for clusterName := range clustersFromDB {
		batchBuilder.Delete(clusterName)
	}
	// batch contains at most number of statements as the number of managed cluster per LH
	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf("failed to perform batch - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}
