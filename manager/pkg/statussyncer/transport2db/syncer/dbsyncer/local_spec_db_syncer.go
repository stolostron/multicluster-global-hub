package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	statusbundle "github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/syncer/dispatcher"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
)

// NewLocalSpecDBSyncer creates a new instance of LocalSpecDBSyncer.
func NewLocalSpecDBSyncer(log logr.Logger, config *corev1.ConfigMap) DBSyncer {
	dbSyncer := &LocalSpecDBSyncer{
		log:                                     log,
		config:                                  config,
		createLocalPolicySpecBundleFunc:         statusbundle.NewLocalPolicySpecBundle,
		createLocalPlacementRulesSpecBundleFunc: statusbundle.NewLocalPlacementRulesBundle,
	}

	log.Info("initialized local spec db syncer")

	return dbSyncer
}

// LocalSpecDBSyncer implements local objects spec db sync business logic.
type LocalSpecDBSyncer struct {
	log                                     logr.Logger
	config                                  *corev1.ConfigMap
	createLocalPolicySpecBundleFunc         status.CreateBundleFunction
	createLocalPlacementRulesSpecBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *LocalSpecDBSyncer) RegisterCreateBundleFunctions(transportDispatcher *dispatcher.TransportDispatcher) {
	predicate := func() bool {
		return syncer.config.Data["enableLocalPolicies"] == "true"
	}

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalPolicySpecMsgKey,
		CreateBundleFunc: syncer.createLocalPolicySpecBundleFunc,
		Predicate:        predicate,
	})

	transportDispatcher.BundleRegister(&registration.BundleRegistration{
		MsgID:            constants.LocalPlacementRulesMsgKey,
		CreateBundleFunc: syncer.createLocalPlacementRulesSpecBundleFunc,
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
func (syncer *LocalSpecDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalPolicySpecPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalPolicySpecBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper(database.LocalPolicySpecTableName)))

	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalPlacementRulesSpecPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalPlacementRulesSpecBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper(database.PlacementRulesTableName)))
}

func (syncer *LocalSpecDBSyncer) handleLocalObjectsBundleWrapper(tableName string) func(ctx context.Context,
	bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
	return func(ctx context.Context, bundle status.Bundle,
		dbClient database.StatusTransportBridgeDB,
	) error {
		return syncer.handleLocalObjectsBundle(ctx, bundle, dbClient, database.LocalSpecSchema, tableName)
	}
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
// if the row isn't in the bundle then delete it.
func (syncer *LocalSpecDBSyncer) handleLocalObjectsBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.LocalPoliciesStatusDB, schema string, tableName string,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	resourceIDToVersionMapFromDB, err := dbClient.GetLocalResourceIDToVersionByLeafHub(ctx, schema, tableName, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s.%s' IDs from db - %w", schema, tableName, err)
	}

	batchBuilder := dbClient.NewGenericLocalBatchBuilder(schema, tableName, leafHubName)

	for _, object := range bundle.GetObjects() {
		specificObj, ok := object.(metav1.Object)
		if !ok {
			continue
		}

		uid := string(specificObj.GetUID())
		resourceVersionFromDB, objInDB := resourceIDToVersionMapFromDB[uid]

		if !objInDB { // object not found in the db table
			batchBuilder.Insert(object)
			continue
		}

		delete(resourceIDToVersionMapFromDB, uid)

		if specificObj.GetResourceVersion() == resourceVersionFromDB {
			continue // update object in db only if what we got is a different (newer) version of the resource.
		}

		batchBuilder.Update(object)
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for uid := range resourceIDToVersionMapFromDB {
		batchBuilder.Delete(uid)
	}

	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf("failed to perform batch - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}
