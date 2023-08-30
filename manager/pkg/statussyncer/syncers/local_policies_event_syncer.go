package dbsyncer

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	corev1 "k8s.io/api/core/v1"

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
	leafHubName := bundle.GetLeafHubName()
	db := database.GetGorm()
	// https://gorm.io/docs/transactions.html
	err := db.Transaction(func(tx *gorm.DB) error {
		if len(bundle.GetObjects()) == 0 {
			return nil
		}
		for _, object := range bundle.GetObjects() {
			policyStatusEvent, ok := object.(*models.LocalClusterPolicyEvent)
			if !ok {
				continue
			}
			err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "event_name"}, {Name: "count"}, {Name: "created_at"}},
				DoNothing: true,
			}).Create(&models.LocalClusterPolicyEvent{
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
			}).Error
			if err != nil {
				return err
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed handling leaf hub LocalPolicyStatusEvent bundle - %w", err)
	}

	logBundleHandlingMessage(syncer.log, bundle, finishBundleHandlingMessage)
	return nil
}
