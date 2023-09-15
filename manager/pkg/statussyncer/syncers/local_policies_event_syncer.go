package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/common"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

func NewLocalPolicyEventSyncer(log logr.Logger) DBSyncer {
	dbSyncer := &localPoliciesStatusEventSyncer{
		log:                                    log,
		createLocalPolicyStatusEventBundleFunc: status.NewClusterPolicyEventBundle,
	}
	log.Info("initialized local policies status event syncer")
	return dbSyncer
}

type localPoliciesStatusEventSyncer struct {
	log                                    logr.Logger
	createLocalPolicyStatusEventBundleFunc status.CreateBundleFunction
}

// RegisterCreateBundleFunctions registers create bundle functions within the transport instance.
func (syncer *localPoliciesStatusEventSyncer) RegisterCreateBundleFunctions(transportDispatcher BundleRegisterable) {
	predicate := func() bool {
		return true
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
func (syncer *localPoliciesStatusEventSyncer) RegisterBundleHandlerFunctions(
	conflationManager *conflator.ConflationManager,
) {
	conflationManager.Register(conflator.NewConflationRegistration(
		conflator.LocalPolicyStatusEventPriority,
		bundle.CompleteStateMode,
		helpers.GetBundleType(syncer.createLocalPolicyStatusEventBundleFunc()),
		syncer.handleLocalObjectsBundleWrapper()))
}

func (syncer *localPoliciesStatusEventSyncer) handleLocalObjectsBundleWrapper() func(
	ctx context.Context, bundle status.Bundle, dbClient postgres.StatusTransportBridgeDB) error {
	return func(ctx context.Context, bundle status.Bundle,
		dbClient postgres.StatusTransportBridgeDB,
	) error {
		return syncer.handleLocalObjectsBundle(ctx, bundle)
	}
}

// handleLocalObjectsBundle generic function to handle bundles of local objects.
// if the row doesn't exist then add it.
// if the row exists then update it.
// if the row isn't in the bundle then delete it.
func (syncer *localPoliciesStatusEventSyncer) handleLocalObjectsBundle(ctx context.Context,
	bundle status.Bundle,
) error {
	logBundleHandlingMessage(syncer.log, bundle, startBundleHandlingMessage)

	if len(bundle.GetObjects()) == 0 {
		return nil
	}

	leafHubName := bundle.GetLeafHubName()
	batchUpsertLocalPolicyEvents := []models.LocalClusterPolicyEvent{}

	for _, object := range bundle.GetObjects() {
		policyStatusEvent, ok := object.(*models.LocalClusterPolicyEvent)
		if !ok {
			continue
		}
		batchUpsertLocalPolicyEvents = append(batchUpsertLocalPolicyEvents, models.LocalClusterPolicyEvent{
			BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
				EventName:   policyStatusEvent.EventName,
				PolicyID:    policyStatusEvent.PolicyID,
				Message:     policyStatusEvent.Message,
				Reason:      policyStatusEvent.Reason,
				LeafHubName: leafHubName,
				Source:      nil,
				Count:       policyStatusEvent.Count,
				Compliance:  string(common.GetDatabaseCompliance(policyStatusEvent.Compliance)),
				CreatedAt:   policyStatusEvent.CreatedAt,
			},
			ClusterID: policyStatusEvent.ClusterID,
		})
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
		DoNothing: true,
	}).CreateInBatches(batchUpsertLocalPolicyEvents, 100).Error
	if err != nil {
		return err
	}
	if err != nil {
		return fmt.Errorf("failed handling leaf hub LocalPolicyStatusEvent bundle - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}
