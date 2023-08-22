package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

// NewLocalSpecPoliciesSyncer creates a new instance of LocalSpecDBSyncer.
func NewLocalSpecPoliciesSyncer(log logr.Logger, config *corev1.ConfigMap) DBSyncer {
	dbSyncer := &localSpecPoliciesSyncer{
		log:                             log,
		config:                          config,
		createLocalPolicySpecBundleFunc: statusbundle.NewLocalPolicySpecBundle,
	}
	log.Info("initialized local spec policies syncer")
	return dbSyncer
}

// localSpecPoliciesSyncer implements local objects spec db sync business logic.
type localSpecPoliciesSyncer struct {
	log                             logr.Logger
	config                          *corev1.ConfigMap
	createLocalPolicySpecBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *localSpecPoliciesSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	predicate := func() bool {
		return syncer.config.Data["enableLocalPolicies"] == "true"
	}

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalPolicySpecMsgKey,
		CreateBundleFunc: syncer.createLocalPolicySpecBundleFunc,
		Predicate:        predicate,
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler functions need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *localSpecPoliciesSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalPolicySpecPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalPolicySpecBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper(database.LocalPolicySpecTableName)))
}

func (syncer *localSpecPoliciesSyncer) handleLocalObjectsBundleWrapper(tableName string) func(ctx context.Context,
	bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
	return func(ctx context.Context, bundle status.Bundle,
		dbClient postgres.StatusTransportBridgeDB,
	) error {
		return syncer.handleLocalObjectsBundle(ctx, bundle, dbClient, database.LocalSpecSchema, tableName)
	}
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
// if the row isn't in the bundle then delete it.
func (syncer *localSpecPoliciesSyncer) handleLocalObjectsBundle(ctx context.Context, bundle status.Bundle,
	dbClient postgres.LocalPoliciesStatusDB, schema string, tableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	db := database.GetGorm()
	policyIdToVersionMapFromDB, err := getPolicyIdToVersionMap(db, schema, tableName, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s.%s' IDs from db - %w", schema, tableName, err)
	}

	// https://gorm.io/docs/transactions.html
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, object := range bundle.GetObjects() {
			specificObj, ok := object.(metav1.Object)
			if !ok {
				continue
			}
			uid := string(specificObj.GetUID())
			resourceVersionFromDB, objInDB := policyIdToVersionMapFromDB[uid]

			payload, err := json.Marshal(specificObj)
			if err != nil {
				return err
			}

			// if the row doesn't exist in db then add it.
			if !objInDB {
				tx.Create(&models.LocalSpecPolicy{
					LeafHubName: leafHubName,
					Payload:     payload,
				})
				continue
			}

			// remove the handled object from the map
			delete(policyIdToVersionMapFromDB, uid)

			// update object only if what we got is a different (newer) version of the resource
			if specificObj.GetResourceVersion() == resourceVersionFromDB {
				continue
			}
			err = tx.Model(&models.LocalSpecPolicy{}).
				Where(&models.LocalSpecPolicy{
					LeafHubName: leafHubName,
					PolicyID:    uid,
				}).
				Updates(models.LocalSpecPolicy{
					Payload:     payload,
					LeafHubName: leafHubName,
				}).Error
			if err != nil {
				return err
			}
		}

		// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
		for uid := range policyIdToVersionMapFromDB {
			// https://gorm.io/docs/delete.html#Soft-Delete
			err = tx.Where(&models.LocalSpecPolicy{
				LeafHubName: leafHubName,
				PolicyID:    uid,
			}).Delete(&models.LocalSpecPolicy{}).Error
			if err != nil {
				return err
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed handling leaf hub '%s.%s' bundle - %w", schema, tableName, err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}

func getPolicyIdToVersionMap(db *gorm.DB, schema, tableName, leafHubName string) (map[string]string, error) {
	var resourceVersions []models.ResourceVersion

	err := db.Select("payload->'metadata'->>'uid' AS key, payload->'metadata'->>'resourceVersion' AS resource_version").
		Where(&models.LocalSpecPolicy{ // Find soft deleted records: db.Unscoped().Where(...).Find(...)
			LeafHubName: leafHubName,
		}).Find(&models.LocalSpecPolicy{}).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}

	idToVersionMap := make(map[string]string)
	for _, resource := range resourceVersions {
		idToVersionMap[resource.Key] = resource.ResourceVersion
	}

	return idToVersionMap, nil
}
