package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	set "github.com/deckarep/golang-set"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

type policyComplianceHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
}

func RegisterPolicyComplianceHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.ComplianceType)
	logName := strings.ReplaceAll(eventType, enum.EventTypePrefix, "")
	h := &policyComplianceHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.CompliancePriority,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

// if we got inside the handler function, then the bundle version is newer than what was already handled.
// handling clusters per policy bundle inserts or deletes rows from/to the compliance table.
// in case the row already exists (leafHubName, policyId, clusterName) it updates the compliance status accordingly.
// this bundle is triggered only when policy was added/removed or when placement rule has changed which caused list of
// clusters (of at least one policy) to change.
// in other cases where only compliance status change, only compliance bundle is received.
func (h *policyComplianceHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	h.log.Debugw(startMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)

	data := grc.ComplianceBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	var compliancesFromDB []models.StatusCompliance
	db := database.GetGorm()
	err := db.Where(&models.StatusCompliance{LeafHubName: leafHubName}).Find(&compliancesFromDB).Error
	if err != nil {
		return err
	}

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allComplianceClustersFromDB, err := getComplianceClusterSets(db, "leaf_hub_name = ?", leafHubName)
	if err != nil {
		return err
	}

	for _, eventCompliance := range data { // every object is clusters list per policy with full state

		policyID := eventCompliance.PolicyID
		complianceClustersFromDB, policyExistsInDB := allComplianceClustersFromDB[policyID]
		if !policyExistsInDB {
			complianceClustersFromDB = NewPolicyClusterSets()
		}

		allClustersOnDB := complianceClustersFromDB.GetAllClusters()
		// handle compliant clusters of the policy
		compliantCompliances := newCompliances(leafHubName, policyID, database.Compliant,
			eventCompliance.CompliantClusters, allClustersOnDB)

		// handle non compliant clusters of the policy
		nonCompliantCompliances := newCompliances(leafHubName, policyID, database.NonCompliant,
			eventCompliance.NonCompliantClusters, allClustersOnDB)

		// handle unknown compliance clusters of the policy
		unknownCompliances := newCompliances(leafHubName, policyID, database.Unknown,
			eventCompliance.UnknownComplianceClusters, allClustersOnDB)

		// handle pending compliance clusters of the policy
		pendingCompliances := newCompliances(leafHubName, policyID, database.Pending,
			eventCompliance.PendingComplianceClusters, allClustersOnDB)

		batchCompliances := []models.StatusCompliance{}
		batchCompliances = append(batchCompliances, compliantCompliances...)
		batchCompliances = append(batchCompliances, nonCompliantCompliances...)
		batchCompliances = append(batchCompliances, unknownCompliances...)
		batchCompliances = append(batchCompliances, pendingCompliances...)

		// batch upsert
		err = db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(batchCompliances, 100).Error
		if err != nil {
			return err
		}

		// delete
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, name := range allClustersOnDB.ToSlice() {
				clusterName, ok := name.(string)
				if !ok {
					continue
				}
				err := tx.Where(&models.StatusCompliance{
					LeafHubName: leafHubName,
					PolicyID:    policyID,
					ClusterName: clusterName,
				}).Delete(&models.StatusCompliance{}).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to handle clusters per policy bundle - %w", err)
		}
		// keep this policy in db, should remove from db only policies that were not sent in the bundle
		delete(allComplianceClustersFromDB, policyID)
	}

	// delete the policy isn't contained on the bundle
	err = db.Transaction(func(tx *gorm.DB) error {
		for policyID := range allComplianceClustersFromDB {
			err := tx.Where(&models.StatusCompliance{
				PolicyID: policyID,
			}).Delete(&models.StatusCompliance{}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle compliance event - %w", err)
	}

	h.log.Debugw(finishMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)
	return nil
}

func newCompliances(leafHub, policyID string, compliance database.ComplianceStatus,
	eventComplianceClusters []string, allClusterOnDB set.Set,
) []models.StatusCompliance {
	compliances := make([]models.StatusCompliance, 0)
	for _, cluster := range eventComplianceClusters {
		compliances = append(compliances, models.StatusCompliance{
			LeafHubName: leafHub,
			PolicyID:    policyID,
			ClusterName: cluster,
			Error:       database.ErrorNone,
			Compliance:  compliance,
		})
		allClusterOnDB.Remove(cluster)
	}
	return compliances
}

func getComplianceClusterSets(db *gorm.DB, query interface{}, args ...interface{}) (
	map[string]*PolicyClustersSets, error,
) {
	var compliancesFromDB []models.StatusCompliance
	err := db.Where(query, args...).Find(&compliancesFromDB).Error
	if err != nil {
		return nil, err
	}

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	policyComplianceRowsFromDB := make(map[string]*PolicyClustersSets)
	for _, compliance := range compliancesFromDB {
		if _, ok := policyComplianceRowsFromDB[compliance.PolicyID]; !ok {
			policyComplianceRowsFromDB[compliance.PolicyID] = NewPolicyClusterSets()
		}
		policyComplianceRowsFromDB[compliance.PolicyID].AddCluster(compliance.ClusterName, compliance.Compliance)
	}
	return policyComplianceRowsFromDB, nil
}

// NewPolicyClusterSets creates a new instance of PolicyClustersSets.
func NewPolicyClusterSets() *PolicyClustersSets {
	return &PolicyClustersSets{
		complianceToSetMap: map[database.ComplianceStatus]set.Set{
			database.Compliant:    set.NewSet(),
			database.NonCompliant: set.NewSet(),
			database.Unknown:      set.NewSet(),
			database.Pending:      set.NewSet(),
		},
	}
}

// PolicyClustersSets is a data structure to hold both non compliant clusters set and unknown clusters set.
type PolicyClustersSets struct {
	complianceToSetMap map[database.ComplianceStatus]set.Set
}

// AddCluster adds the given cluster name to the given compliance status clusters set.
func (sets *PolicyClustersSets) AddCluster(clusterName string, complianceStatus database.ComplianceStatus) {
	sets.complianceToSetMap[complianceStatus].Add(clusterName)
}

// GetAllClusters returns the clusters set of a policy (union of compliant/nonCompliant/unknown clusters).
func (sets *PolicyClustersSets) GetAllClusters() set.Set {
	return sets.complianceToSetMap[database.Compliant].
		Union(sets.complianceToSetMap[database.NonCompliant].
			Union(sets.complianceToSetMap[database.Pending]).
			Union(sets.complianceToSetMap[database.Unknown]))
}

// GetClusters returns the clusters set by compliance status.
func (sets *PolicyClustersSets) GetClusters(complianceStatus database.ComplianceStatus) set.Set {
	return sets.complianceToSetMap[complianceStatus]
}
