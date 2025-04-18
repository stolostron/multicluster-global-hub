package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type localPolicyCompleteHandler struct {
	log            *zap.SugaredLogger
	eventType      string
	dependencyType string
	eventSyncMode  enum.EventSyncMode
	eventPriority  conflator.ConflationPriority
}

func RegisterLocalPolicyCompleteHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalCompleteComplianceType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &localPolicyCompleteHandler{
		log:            logger.ZapLogger(logName),
		eventType:      eventType,
		dependencyType: string(enum.LocalComplianceType),
		eventSyncMode:  enum.CompleteStateMode,
		eventPriority:  conflator.LocalCompleteCompliancePriority,
	}

	registration := conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEventWrapper,
	)
	registration.WithDependency(dependency.NewDependency(h.dependencyType, dependency.ExactMatch))
	conflationManager.Register(registration)
}

func (h *localPolicyCompleteHandler) handleEventWrapper(ctx context.Context, evt *cloudevents.Event) error {
	return handleCompleteCompliance(h.log, ctx, evt)
}

func handleCompleteCompliance(log *zap.SugaredLogger, ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHub := evt.Source()
	log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

	db := database.GetGorm()

	// policyID: {  nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allCompleteRowsFromDB, err := getLocalComplianceClusterSets(db, "leaf_hub_name = ? AND compliance <> ?",
		leafHub, database.Compliant)
	if err != nil {
		return err
	}

	data := grc.CompleteComplianceBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	for _, eventCompliance := range data { // every object in bundle is policy compliance status

		policyID := eventCompliance.PolicyID
		policyNamespacedName := eventCompliance.NamespacedName
		// nonCompliantClusters includes both non Compliant and Unknown clusters
		nonComplianceClusterSetsFromDB, policyExistsInDB := allCompleteRowsFromDB[policyID]
		if !policyExistsInDB {
			nonComplianceClusterSetsFromDB = NewPolicyClusterSets()
		}

		allNonComplianceCluster := nonComplianceClusterSetsFromDB.GetAllClusters()
		batchLocalCompliance := []models.LocalStatusCompliance{}

		// nonCompliant: go over the non compliant clusters from event
		for _, eventCluster := range eventCompliance.NonCompliantClusters {
			if !nonComplianceClusterSetsFromDB.GetClusters(database.NonCompliant).Contains(eventCluster) {
				batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
					PolicyID:             policyID,
					PolicyNamespacedName: policyNamespacedName,
					LeafHubName:          leafHub,
					ClusterName:          eventCluster,
					Compliance:           database.NonCompliant,
					Error:                database.ErrorNone,
				})
			}
			allNonComplianceCluster.Remove(eventCluster) // mark cluster as handled
		}

		// pending: go over the pending clusters from event
		for _, eventCluster := range eventCompliance.PendingComplianceClusters {
			if !nonComplianceClusterSetsFromDB.GetClusters(database.Pending).Contains(eventCluster) {
				batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
					PolicyID:             policyID,
					PolicyNamespacedName: policyNamespacedName,
					LeafHubName:          leafHub,
					ClusterName:          eventCluster,
					Compliance:           database.Pending,
					Error:                database.ErrorNone,
				})
			}
			allNonComplianceCluster.Remove(eventCluster) // mark cluster as handled
		}

		// unknown: go over the unknown clusters from event
		for _, eventCluster := range eventCompliance.UnknownComplianceClusters {
			if !nonComplianceClusterSetsFromDB.GetClusters(database.Unknown).Contains(eventCluster) {
				batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
					PolicyID:             policyID,
					PolicyNamespacedName: policyNamespacedName,
					LeafHubName:          leafHub,
					ClusterName:          eventCluster,
					Compliance:           database.Unknown,
					Error:                database.ErrorNone,
				})
			}
			allNonComplianceCluster.Remove(eventCluster) // mark cluster as handled
		}

		// compliant
		for _, name := range allNonComplianceCluster.ToSlice() {
			clusterName, ok := name.(string)
			if !ok {
				continue
			}
			batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
				PolicyID:             policyID,
				PolicyNamespacedName: policyNamespacedName,
				LeafHubName:          leafHub,
				ClusterName:          clusterName,
				Compliance:           database.Compliant,
				Error:                database.ErrorNone,
			})
		}

		err = db.Transaction(func(tx *gorm.DB) error {
			for _, compliance := range batchLocalCompliance {
				e := tx.Updates(compliance).Error
				if e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to update compliances by complete event - %w", err)
		}

		// for policies that are found in the db but not in the bundle - all clusters are Compliant (implicitly)
		delete(allCompleteRowsFromDB, policyID)
	}

	// update policies not in the event - all is Compliant
	err = db.Transaction(func(tx *gorm.DB) error {
		for policyID := range allCompleteRowsFromDB {
			err := tx.Model(&models.LocalStatusCompliance{}).
				Where("policy_id = ? AND leaf_hub_name = ?", policyID, leafHub).
				Updates(&models.LocalStatusCompliance{Compliance: database.Compliant}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed deleting compliances from local complainces - %w", err)
	}

	log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}
