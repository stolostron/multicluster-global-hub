package dbsyncer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

func NewLocalPoliciesStatusEventSyncer(log logr.Logger, config *corev1.ConfigMap) DBSyncer {
	dbSyncer := &localPoliciesStatusEventSyncer{
		log:                                    log,
		config:                                 config,
		createLocalPolicyStatusEventBundleFunc: status.NewClusterPolicyStatusEventBundle,
	}
	log.Info("initialized local policies status event syncer")
	return dbSyncer
}

type localPoliciesStatusEventSyncer struct {
	log                                    logr.Logger
	config                                 *corev1.ConfigMap
	createLocalPolicyStatusEventBundleFunc status.CreateBundleFunction
}

// TODO
// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *localPoliciesStatusEventSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	predicate := func() bool {
		return syncer.config.Data["enableLocalPolicies"] == "true"
	}

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalClusterPolicyStatusEventMsgKey,
		CreateBundleFunc: syncer.createLocalPolicyStatusEventBundleFunc,
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
func (syncer *localPoliciesStatusEventSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalPolicyStatusEventPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalPolicyStatusEventBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper(database.LocalPolicySpecTableName)))
}

func (syncer *localPoliciesStatusEventSyncer) handleLocalObjectsBundleWrapper(tableName string) func(
	ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
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
func (syncer *localPoliciesStatusEventSyncer) handleLocalObjectsBundle(ctx context.Context, bundle status.Bundle,
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
			tx.Model(&models.LocalSpecPolicy{}).
				Where(&models.LocalSpecPolicy{
					LeafHubName: leafHubName,
					PolicyID:    uid,
				}).
				Updates(models.LocalSpecPolicy{
					Payload:     payload,
					LeafHubName: leafHubName,
				})
		}

		// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
		for uid := range policyIdToVersionMapFromDB {
			// https://gorm.io/docs/delete.html#Soft-Delete
			tx.Where(&models.LocalSpecPolicy{
				LeafHubName: leafHubName,
				PolicyID:    uid,
			}).Delete(&models.LocalSpecPolicy{})
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
