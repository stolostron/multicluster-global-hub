package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/dependency"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type localPolicyCompleteHandler struct {
	log            *zap.SugaredLogger
	eventType      string
	dependencyType string
	eventSyncMode  enum.EventSyncMode
	eventPriority  conflator.ConflationPriority
	requester      transport.Requester
}

func RegisterLocalPolicyCompleteHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalCompleteComplianceType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &localPolicyCompleteHandler{
		log:            logger.ZapLogger(logName),
		eventType:      eventType,
		dependencyType: string(enum.LocalComplianceType),
		eventSyncMode:  enum.CompleteStateMode,
		eventPriority:  conflator.LocalCompleteCompliancePriority,
		requester:      conflationManager.Requster,
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
	return h.handleCompleteCompliance(h.log, ctx, evt)
}

func (h *localPolicyCompleteHandler) handleCompleteCompliance(log *zap.SugaredLogger,
	ctx context.Context, evt *cloudevents.Event,
) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHub := evt.Source()
	log.Infow("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

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
		var policyNamespacedName string
		if configs.IsInventoryAPIEnabled() {
			// If inventory enabled, we need to make sure the policy spec info is handled firstly.
			policyNamespacedName, err = getPolicyNamespacedName(db, policyID)
			if err != nil || policyNamespacedName == "" {
				return fmt.Errorf("failed to get policy namespaced name - %v: %w", policyID, err)
			}
		}
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
					PolicyID:    policyID,
					LeafHubName: leafHub,
					ClusterName: eventCluster,
					Compliance:  database.NonCompliant,
					Error:       database.ErrorNone,
				})
			}
			allNonComplianceCluster.Remove(eventCluster) // mark cluster as handled
		}

		// pending: go over the pending clusters from event
		for _, eventCluster := range eventCompliance.PendingComplianceClusters {
			if !nonComplianceClusterSetsFromDB.GetClusters(database.Pending).Contains(eventCluster) {
				batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
					PolicyID:    policyID,
					LeafHubName: leafHub,
					ClusterName: eventCluster,
					Compliance:  database.Pending,
					Error:       database.ErrorNone,
				})
			}
			allNonComplianceCluster.Remove(eventCluster) // mark cluster as handled
		}

		// unknown: go over the unknown clusters from event
		for _, eventCluster := range eventCompliance.UnknownComplianceClusters {
			if !nonComplianceClusterSetsFromDB.GetClusters(database.Unknown).Contains(eventCluster) {
				batchLocalCompliance = append(batchLocalCompliance, models.LocalStatusCompliance{
					PolicyID:    policyID,
					LeafHubName: leafHub,
					ClusterName: eventCluster,
					Compliance:  database.Unknown,
					Error:       database.ErrorNone,
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
				PolicyID:    policyID,
				LeafHubName: leafHub,
				ClusterName: clusterName,
				Compliance:  database.Compliant,
				Error:       database.ErrorNone,
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
		if configs.IsInventoryAPIEnabled() {
			err = syncInventory(h.requester, leafHub,
				models.ResourceVersion{
					Key:  policyID,
					Name: policyNamespacedName,
				},
				batchLocalCompliance,
				nonComplianceClusterSetsFromDB.complianceToSetMap,
				nil,
			)
			if err != nil {
				return fmt.Errorf("failed syncing inventory - %w", err)
			}
		}
		// for policies that are found in the db but not in the bundle - all clusters are Compliant (implicitly)
		delete(allCompleteRowsFromDB, policyID)
	}

	// update policies not in the event - all is Compliant
	return db.Transaction(func(tx *gorm.DB) error {
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
}
