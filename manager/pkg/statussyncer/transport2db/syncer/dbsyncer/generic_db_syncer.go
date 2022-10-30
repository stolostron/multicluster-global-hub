package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// genericDBSyncer implements generic status resource db sync business logic.
type genericDBSyncer struct {
	log             logr.Logger
	transportMsgKey string

	dbSchema    string
	dbTableName string

	createBundleFunc func() status.Bundle
	bundlePriority   conflator.ConflationPriority
	bundleSyncMode   bundle.BundleSyncMode
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *genericDBSyncer) RegisterCreateBundleFunctions(transportInstance transport.Transport) {
	transportInstance.BundleRegister(&registration.BundleRegistration{
		MsgID:            syncer.transportMsgKey,
		CreateBundleFunc: syncer.createBundleFunc,
		Predicate:        func() bool { return true }, // always get generic status resources
	})
}

// RegisterBundleHandlerFunctions registers bundle handler functions within the conflation manager.
// handler function need to do "diff" between objects received in the bundle and the objects in database.
// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
// objects were deleted.
// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
// and if the object was changed, update the db with the current object.
func (syncer *genericDBSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		syncer.bundlePriority,
		syncer.bundleSyncMode,
		helpers.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle status.Bundle, dbClient database.StatusTransportBridgeDB) error {
			return syncer.handleResourcesBundle(ctx, bundle, dbClient)
		},
	))
}

func (syncer *genericDBSyncer) handleResourcesBundle(ctx context.Context, bundle status.Bundle,
	dbClient database.GenericStatusResourceDB,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	idToVersionMapFromDB, err := dbClient.GetResourceIDToVersionByLeafHub(ctx, syncer.dbSchema,
		syncer.dbTableName, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s.%s' IDs from db - %w",
			syncer.dbSchema, syncer.dbTableName, err)
	}

	batchBuilder := dbClient.NewGenericBatchBuilder(syncer.dbSchema, syncer.dbTableName, leafHubName)

	for _, object := range bundle.GetObjects() {
		specificObj, ok := object.(metav1.Object)
		if !ok {
			continue
		}

		uid := getGenericResourceUID(specificObj)
		resourceVersionFromDB, objExistsInDB := idToVersionMapFromDB[uid]

		if !objExistsInDB { // object not found in the db table
			batchBuilder.Insert(uid, object)
			continue
		}

		delete(idToVersionMapFromDB, uid)

		if specificObj.GetResourceVersion() == resourceVersionFromDB {
			continue // update object in db only if what we got is a different (newer) version of the resource.
		}

		batchBuilder.Update(uid, object)
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	for uid := range idToVersionMapFromDB {
		batchBuilder.Delete(uid)
	}

	if err := dbClient.SendBatch(ctx, batchBuilder.Build()); err != nil {
		return fmt.Errorf("failed to perform batch - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)

	return nil
}

func getGenericResourceUID(resourceObject metav1.Object) string {
	if originOwnerReference, found := resourceObject.GetAnnotations()[constants.OriginOwnerReferenceAnnotation]; found {
		// safe if GetAnnotations() returns nil
		return originOwnerReference
	}

	return string(resourceObject.GetUID())
}
