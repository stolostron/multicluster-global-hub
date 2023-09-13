package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cenkalti/backoff/v4"
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
	clusterIdToResourceVersionMapFromDB, err := getClusterIdToVersionMap(db, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub managed clusters from db - %w", err)
	}

	// upinsert cluster
	batchSize := 10
	batchClusters := make([]*clusterv1.ManagedCluster, 0)
	for index, object := range bundle.GetObjects() {
		cluster, ok := object.(*clusterv1.ManagedCluster)
		if !ok {
			continue
		}
		batchClusters := append(batchClusters, cluster)

		// start a transaction when the up to batchsize or process the last cluster
		if len(batchClusters) == batchSize || (len(batchClusters) > 0 && index == (len(batchClusters)-1)) {
			err = backoff.Retry(func() error {
				return syncer.updateClustersWithTransaction(db, leafHubName,
					batchClusters, clusterIdToResourceVersionMapFromDB)
			}, backoff.NewExponentialBackOff())
			if err != nil {
				return err
			}
			batchClusters = make([]*clusterv1.ManagedCluster, 0)
		}
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	err = backoff.Retry(func() error {
		return db.Transaction(func(tx *gorm.DB) error {
			for clusterId := range clusterIdToResourceVersionMapFromDB {
				// https://gorm.io/docs/delete.html#Soft-Delete
				err := tx.Where(&models.ManagedCluster{
					// LeafHubName: leafHubName,
					ClusterID: clusterId,
				}).Delete(&models.ManagedCluster{}).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
	}, backoff.NewExponentialBackOff())
	if err != nil {
		return fmt.Errorf("failed delete objects from database - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func (syncer *ManagedClustersDBSyncer) updateClustersWithTransaction(db *gorm.DB, leafHubName string,
	bundleClusters []*clusterv1.ManagedCluster, clusterRVsFromDB map[string]string,
) error {
	// https://gorm.io/docs/transactions.html
	return db.Transaction(func(tx *gorm.DB) error {
		for _, cluster := range bundleClusters {
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

			payload, err := json.Marshal(cluster)
			if err != nil {
				return err
			}

			clusterVersionFromDB, exist := clusterRVsFromDB[clusterId]
			if !exist { // cluster not found in the db table
				syncer.log.Info("cluster created", "leafHubName", leafHubName, "clusterId", clusterId)
				if err := tx.Save(&models.ManagedCluster{
					ClusterID:   clusterId,
					LeafHubName: leafHubName,
					Payload:     payload,
					Error:       database.ErrorNone,
				}).Error; err != nil {
					return err
				}
				continue
			}

			// remove the handled object from the map
			delete(clusterRVsFromDB, clusterId)

			if cluster.GetResourceVersion() == clusterVersionFromDB {
				continue // update cluster in db only if what we got is a different (newer) version of the resource
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
		}

		return nil
	})
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
