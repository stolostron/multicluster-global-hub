package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// NewManagedClustersDBSyncer creates a new instance of ManagedClustersDBSyncer.
func NewManagedClustersDBSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &ManagedClustersDBSyncer{
		log:              log,
		createBundleFunc: statusbundle.NewManagedClustersStatusBundle,
	}

	log.Info("initialized managed clusters db syncer")
	return dbSyncer
}

// ManagedClustersDBSyncer implements managed clusters db sync business logic.
type ManagedClustersDBSyncer struct {
	log              logr.Logger
	createBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *ManagedClustersDBSyncer) RegisterCreateBundleFunctions(dispatcher BundleRegisterable) {
	dispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.ManagedClustersMsgKey,
		CreateBundleFunc: syncer.createBundleFunc,
		Predicate:        func() bool { return true }, // always get managed clusters bundles
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler function need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *ManagedClustersDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.ManagedClustersPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
			return syncer.handleManagedClustersBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *ManagedClustersDBSyncer) handleManagedClustersBundle(ctx context.Context, bundle status.Bundle,
	dbClient postgres.ManagedClustersStatusDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	db := database.GetGorm()
	clusterIdToVersionMapFromDB, err := getClusterIdToVersionMap(db, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub managed clusters from db - %w", err)
	}

	var wg sync.WaitGroup // Create a WaitGroup

	for _, object := range bundle.GetObjects() {
		cluster, ok := object.(*clusterv1.ManagedCluster)
		if !ok {
			continue
		}

		// Initially, if the clusterID is not exist we will skip it until we get it from ClusterClaim
		clusterId := ""
		for _, claim := range cluster.Status.ClusterClaims {
			if claim.Name == "id.k8s.io" {
				clusterId = claim.Value
				break
			}
		}
		if clusterId == "" {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			// https://gorm.io/docs/transactions.html
			err = db.Transaction(func(tx *gorm.DB) error {
				payload, err := json.Marshal(cluster)
				if err != nil {
					return err
				}

				clusterVersionFromDB, exist := clusterIdToVersionMapFromDB[clusterId]
				if !exist { // cluster not found in the db table
					syncer.log.Info("cluster created", "leafHubName", leafHubName, "clusterId", clusterId)
					tx.Unscoped().Where(&models.ManagedCluster{
						ClusterID: clusterId,
					}).Delete(&models.ManagedCluster{})
					tx.Create(&models.ManagedCluster{
						ClusterID:   clusterId,
						LeafHubName: leafHubName,
						Payload:     payload,
						Error:       database.ErrorNone,
					})
					return nil
				}

				// remove the handled object from the map
				delete(clusterIdToVersionMapFromDB, clusterId)

				if cluster.GetResourceVersion() == clusterVersionFromDB {
					return nil // update cluster in db only if what we got is a different (newer) version of the resource
				}

				syncer.log.Info("cluster updated", "leafHubName", leafHubName, "clusterId", clusterId)
				tx.Model(&models.ManagedCluster{}).
					Where(&models.ManagedCluster{
						ClusterID: clusterId,
					}).
					Updates(models.ManagedCluster{
						Payload:     payload,
						LeafHubName: leafHubName,
					})

					// return nil will commit the whole transaction
				return nil
			})
			if err != nil {
				syncer.log.Error(err, "failed handling managed clusters bundle", "clusterID", clusterId)
			}
		}()
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
		for clusterId := range clusterIdToVersionMapFromDB {
			// https://gorm.io/docs/delete.html#Soft-Delete
			tx.Where(&models.ManagedCluster{
				LeafHubName: leafHubName,
				ClusterID:   clusterId,
			}).Delete(&models.ManagedCluster{})
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed handling managed clusters bundle - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func getClusterIdToVersionMap(db *gorm.DB, leafHubName string) (map[string]string, error) {
	var resourceVersions []models.ResourceVersion

	err := db.Select("cluster_id AS key, payload->'metadata'->>'resourceVersion' AS resource_version").
		Where(&models.ManagedCluster{
			LeafHubName: leafHubName,
		}).Find(&models.ManagedCluster{}).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}
	nameToVersionMap := make(map[string]string)
	for _, resource := range resourceVersions {
		nameToVersionMap[resource.Key] = resource.ResourceVersion
	}
	return nameToVersionMap, nil
}
