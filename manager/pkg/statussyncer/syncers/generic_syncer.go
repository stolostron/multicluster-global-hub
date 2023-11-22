package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/dao"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/registration"
)

const (
	startBundleHandlingMessage  = "started handling bundle"
	finishBundleHandlingMessage = "finished handling bundle"
)

// genericStatusSyncer implements generic status resource db sync business logic.
// only for the table with: id, leaf_hub_name and payload
type genericStatusSyncer struct {
	log             logr.Logger
	transportMsgKey string

	dbSchema    string
	dbTableName string

	createBundleFunc func() bundle.ManagerBundle
	bundlePriority   conflator.ConflationPriority
	bundleSyncMode   metadata.BundleSyncMode
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *genericStatusSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	transportDispatcher.BundleRegister(&registration.BundleRegistration{
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
func (syncer *genericStatusSyncer) RegisterBundleHandlerFunctions(conflationManager *conflator.ConflationManager) {
	conflationManager.Register(conflator.NewConflationRegistration(
		syncer.bundlePriority,
		syncer.bundleSyncMode,
		bundle.GetBundleType(syncer.createBundleFunc()),
		func(ctx context.Context, bundle bundle.ManagerBundle) error {
			return syncer.handleResourcesBundle(ctx, bundle)
		},
	))
}

func (syncer *genericStatusSyncer) handleResourcesBundle(ctx context.Context, bundle bundle.ManagerBundle) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)
	leafHubName := bundle.GetLeafHubName()

	db := database.GetGorm()
	genericDao := dao.NewGenericDao(db, fmt.Sprintf("%s.%s", syncer.dbSchema, syncer.dbTableName))
	idToVersionMapFromDB, err := genericDao.GetIdToVersionByHub(leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s.%s' IDs from db - %w",
			syncer.dbSchema, syncer.dbTableName, err)
	}

	// using the transaction for now. once the unique is fixed, try to upInsert with batch operations
	err = db.Transaction(func(tx *gorm.DB) error {
		genericDao := dao.NewGenericDao(tx, fmt.Sprintf("%s.%s", syncer.dbSchema, syncer.dbTableName))
		for _, object := range bundle.GetObjects() {
			specificObj, ok := object.(metav1.Object)
			if !ok {
				continue
			}
			uid := getGenericResourceUID(specificObj)
			resourceVersionFromDB, objExistsInDB := idToVersionMapFromDB[uid]

			if !objExistsInDB { // object not found in the db table
				if e := genericDao.Insert(leafHubName, uid, object); e != nil {
					return e
				}
				continue
			}

			delete(idToVersionMapFromDB, uid)

			if specificObj.GetResourceVersion() == resourceVersionFromDB {
				continue // update object in db only if what we got is a different (newer) version of the resource.
			}

			if e := genericDao.Update(leafHubName, uid, object); e != nil {
				return e
			}
		}

		// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
		for uid := range idToVersionMapFromDB {
			if e := genericDao.Delete(leafHubName, uid); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
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

func logBundleHandlingMessage(log logr.Logger, b bundle.ManagerBundle, message string) {
	log.V(2).Info(message, "BundleType", bundle.GetBundleType(b), "LeafHubName", b.GetLeafHubName(),
		"Version", b.GetVersion().String())
}
