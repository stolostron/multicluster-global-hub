package generic

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/dao"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

const (
	startMessage  = "start handling cloudevent"
	finishMessage = "finish handing cloudevent"
)

// genericObjectHandler implements generic status resource db sync business logic.
// only for the table with: id, leaf_hub_name and payload
type genericObjectHandler[T metav1.Object] struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	table         string
}

func RegisterGenericHandler[T metav1.Object](conflationManager *conflator.ConflationManager,
	eventType string, priority conflator.ConflationPriority, syncMode enum.EventSyncMode, table string,
) {
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &genericObjectHandler[T]{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: syncMode,
		eventPriority: priority,
		table:         table,
	}

	// register bundle handler function within the conflation manager.
	// handler function need to do "diff" between objects received in the bundle and the objects in database.
	// leaf hub sends only the current existing objects, and status transport bridge should understand implicitly which
	// objects were deleted.
	// therefore, whatever is in the db and cannot be found in the bundle has to be deleted from the database.
	// for the objects that appear in both, need to check if something has changed using resourceVersion field comparison
	// and if the object was changed, update the db with the current object.
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *genericObjectHandler[T]) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[version.ExtVersion]
	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	leafHubName := evt.Source()
	var data []T
	e := evt.DataAs(&data)
	if e != nil {
		return fmt.Errorf("failed to parse the event data: %v", e)
	}

	// get the exist objects in database
	db := database.GetGorm()
	genericDao := dao.NewGenericDao(db, h.table)
	idToVersionMapFromDB, err := genericDao.GetIdToVersionByHub(leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub '%s' IDs from db - %w", h.table, err)
	}

	// using the transaction for now. once the unique is fixed, try to upInsert with batch operations
	err = db.Transaction(func(tx *gorm.DB) error {
		genericDao := dao.NewGenericDao(tx, h.table)

		for _, object := range data {
			specificObj := object
			uid := getGenericObjectUID(specificObj)
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

	h.log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func getGenericObjectUID(resourceObject metav1.Object) string {
	return string(resourceObject.GetUID())
}
