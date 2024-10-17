package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

const (
	startMessage  = "handler start"
	finishMessage = "handler finished"
)

type localPolicySpecHandler struct {
	log           logr.Logger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterLocalPolicySpecHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalPolicySpecType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &localPolicySpecHandler{
		log:           ctrl.Log.WithName(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.LocalPolicySpecPriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
// if the row isn't in the bundle then delete it.
func (h *localPolicySpecHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.V(2).Info(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	db := database.GetGorm()
	policyIdToVersionMapFromDB, err := getPolicyIdToVersionMap(db, leafHubName)
	if err != nil {
		return err
	}

	var data []policiesv1.Policy
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	batchLocalPolicySpec := []models.LocalSpecPolicy{}
	for _, object := range data {
		specificObj := object
		uid := string(specificObj.GetUID())
		resourceVersionFromDB, objInDB := policyIdToVersionMapFromDB[uid]

		payload, err := json.Marshal(specificObj)
		if err != nil {
			return err
		}
		// if the row doesn't exist in db then add it.
		if !objInDB {
			batchLocalPolicySpec = append(batchLocalPolicySpec, models.LocalSpecPolicy{
				PolicyID:    uid,
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
		batchLocalPolicySpec = append(batchLocalPolicySpec, models.LocalSpecPolicy{
			PolicyID:    uid,
			LeafHubName: leafHubName,
			Payload:     payload,
		})
	}

	err = db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchLocalPolicySpec, 100).Error
	if err != nil {
		return err
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	// https://gorm.io/docs/transactions.html
	// https://gorm.io/docs/delete.html#Soft-Delete
	err = db.Transaction(func(tx *gorm.DB) error {
		for uid := range policyIdToVersionMapFromDB {
			err = tx.Where(&models.LocalSpecPolicy{
				PolicyID: uid,
			}).Delete(&models.LocalSpecPolicy{}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed deleting records from local_spec.policies - %w", err)
	}

	h.log.V(2).Info(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func getPolicyIdToVersionMap(db *gorm.DB, leafHubName string) (map[string]string, error) {
	var resourceVersions []models.ResourceVersion

	// Find soft deleted records: db.Unscoped().Where(...).Find(...)
	err := db.Select("payload->'metadata'->>'uid' AS key, payload->'metadata'->>'resourceVersion' AS resource_version").
		Where(&models.LocalSpecPolicy{ // Find soft deleted records: db.Unscoped().Where(...).Find(...)
			LeafHubName: leafHubName,
		}).
		Find(&models.LocalSpecPolicy{}).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}

	idToVersionMap := make(map[string]string)
	for _, resource := range resourceVersions {
		idToVersionMap[resource.Key] = resource.ResourceVersion
	}

	return idToVersionMap, nil
}
