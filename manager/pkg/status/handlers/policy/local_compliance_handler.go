package policy

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	set "github.com/deckarep/golang-set"
	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/log"
	kesselv1betarelations "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/relationships"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type localPolicyComplianceHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	requester     transport.Requester
}

func RegisterLocalPolicyComplianceHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalComplianceType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &localPolicyComplianceHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.LocalCompliancePriority,
		requester:     conflationManager.Requster,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEventWrapper,
	))
}

func (h *localPolicyComplianceHandler) handleEventWrapper(ctx context.Context, evt *cloudevents.Event) error {
	return h.handleCompliance(ctx, evt)
}

func (h *localPolicyComplianceHandler) handleCompliance(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHub := evt.Source()
	h.log.Info("handler start ", "type ", evt.Type(), "LH ", evt.Source(), "version ", version)

	data := grc.ComplianceBundle{}
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	db := database.GetGorm()
	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allComplianceClustersFromDB, err := getLocalComplianceClusterSets(db, "leaf_hub_name = ?", leafHub)
	if err != nil {
		return err
	}

	for _, eventCompliance := range data { // every object is clusters list per policy with full state
		policyID := eventCompliance.PolicyID

		if configs.IsInventoryAPIEnabled() {
			// If inventory enabled, we need to make sure the policy spec info is handled firstly.
			policyNamespacedName, err := getPolicyNamespacedName(db, policyID)
			if err != nil || policyNamespacedName == "" {
				return fmt.Errorf("failed to get policy namespaced name -%v: %w", policyID, err)
			}
		}

		log.Debug("handle clusters per policy bundle", "policyID", policyID)
		complianceClustersFromDB, policyExistsInDB := allComplianceClustersFromDB[policyID]
		if !policyExistsInDB {
			complianceClustersFromDB = NewPolicyClusterSets()
		}

		allClustersOnDB := complianceClustersFromDB.GetAllClusters()

		// handle compliant clusters of the policy
		compliantCompliances := newLocalCompliances(leafHub, policyID, database.Compliant,
			eventCompliance.CompliantClusters, allClustersOnDB)

		// handle non compliant clusters of the policy
		nonCompliantCompliances := newLocalCompliances(leafHub, policyID, database.NonCompliant,
			eventCompliance.NonCompliantClusters, allClustersOnDB)

		// handle unknown compliance clusters of the policy
		unknownCompliances := newLocalCompliances(leafHub, policyID, database.Unknown,
			eventCompliance.UnknownComplianceClusters, allClustersOnDB)

		// handle pending compliance clusters of the policy
		pendingCompliances := newLocalCompliances(leafHub, policyID, database.Pending,
			eventCompliance.PendingComplianceClusters, allClustersOnDB)

		batchLocalCompliances := []models.LocalStatusCompliance{}
		batchLocalCompliances = append(batchLocalCompliances, compliantCompliances...)
		batchLocalCompliances = append(batchLocalCompliances, nonCompliantCompliances...)
		batchLocalCompliances = append(batchLocalCompliances, unknownCompliances...)
		batchLocalCompliances = append(batchLocalCompliances, pendingCompliances...)

		// batch upsert
		err = db.Clauses(clause.OnConflict{
			UpdateAll: true,
		}).CreateInBatches(batchLocalCompliances, 100).Error
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
				err := tx.Where(&models.LocalStatusCompliance{
					LeafHubName: leafHub,
					PolicyID:    policyID,
					ClusterName: clusterName,
				}).Delete(&models.LocalStatusCompliance{}).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed to handle clusters per policy bundle - %w", err)
		}

		if configs.IsInventoryAPIEnabled() {
			log.Debugf("sync to inventory api - %s", policyID)
			err = syncInventory(ctx, db, h.log, h.requester, leafHub, policyID, batchLocalCompliances,
				complianceClustersFromDB.complianceToSetMap,
				allClustersOnDB,
			)
			if err != nil {
				return fmt.Errorf("failed syncing inventory - %w", err)
			}
		}
		// keep this policy in db, should remove from db only policies that were not sent in the bundle
		delete(allComplianceClustersFromDB, policyID)
	}

	/* Delete the inventory data in local_policy_spec_handler.go*/

	// delete the policy isn't contained on the bundle
	err = db.Transaction(func(tx *gorm.DB) error {
		for policyID := range allComplianceClustersFromDB {
			err := tx.Where(&models.LocalStatusCompliance{
				PolicyID: policyID,
			}).Delete(&models.LocalStatusCompliance{}).Error
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to handle local compliance event - %w", err)
	}

	h.log.Info("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

// syncInventory generate the create/update/delete compliances
// and post the data to database and inventory
func syncInventory(
	ctx context.Context,
	db *gorm.DB,
	log *zap.SugaredLogger,
	requester transport.Requester,
	leafHubName string,
	policyID string,
	localCompliancesFromEvents []models.LocalStatusCompliance,
	complianceFromDb map[database.ComplianceStatus]set.Set,
	allClustersOnDB set.Set,
) error {
	createCompliances, updateCompliances, deleteCompliances := generateCreateUpdateDeleteCompliances(
		localCompliancesFromEvents,
		complianceFromDb,
		allClustersOnDB,
		leafHubName,
		policyID,
	)

	log.Debugw("post compliances data", "LH",
		leafHubName, "policyID", policyID, "create",
		createCompliances, "update",
		updateCompliances, "delete",
		deleteCompliances)
	if requester == nil {
		return fmt.Errorf("requester is nil")
	}
	clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	log.Debugf("clusterInfo: %v", clusterInfo)
	if err != nil || clusterInfo.MchVersion == "" {
		log.Warnf("failed to get cluster info from db - %v", err)
	}
	return postCompliancesToInventoryApi(
		ctx,
		db,
		log,
		requester,
		leafHubName,
		createCompliances,
		updateCompliances,
		deleteCompliances,
		clusterInfo.MchVersion,
	)
}

func postCompliancesToInventoryApi(
	ctx context.Context,
	db *gorm.DB,
	log *zap.SugaredLogger,
	requester transport.Requester,
	leafHub string,
	createCompliances []models.LocalStatusCompliance,
	updateCompliances []models.LocalStatusCompliance,
	deleteCompliances []models.LocalStatusCompliance,
	mchVersion string,
) error {
	if len(createCompliances) > 0 {
		for _, createCompliance := range createCompliances {
			policyNamespacedName, err := getPolicyNamespacedName(db, createCompliance.PolicyID)
			if err != nil || policyNamespacedName == "" {
				log.Errorf("failed to get policy namespaced name -%v: %w", createCompliance.PolicyID, err)
				continue
			}
			if resp, err := requester.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
				UpdateK8SPolicyIsPropagatedToK8SCluster(ctx, updateK8SPolicyIsPropagatedToK8SCluster(
					policyNamespacedName, createCompliance.ClusterName, string(createCompliance.Compliance),
					leafHub, mchVersion)); err != nil {
				log.Errorf("failed to create k8s policy is propagated to k8s cluster -%v: %w", resp, err)
			}
		}
	}
	if len(updateCompliances) > 0 {
		for _, updateCompliance := range updateCompliances {
			policyNamespacedName, err := getPolicyNamespacedName(db, updateCompliance.PolicyID)
			if err != nil || policyNamespacedName == "" {
				log.Errorf("failed to get policy namespaced name -%v: %w", updateCompliance.PolicyID, err)
				continue
			}
			if resp, err := requester.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
				UpdateK8SPolicyIsPropagatedToK8SCluster(ctx, updateK8SPolicyIsPropagatedToK8SCluster(
					policyNamespacedName, updateCompliance.ClusterName, string(updateCompliance.Compliance),
					leafHub, mchVersion)); err != nil {
				log.Errorf("failed to update k8s policy is propagated to k8s cluster -%v: %w", resp, err)
			}
		}
	}
	if len(deleteCompliances) > 0 {
		for _, deleteCompliance := range deleteCompliances {
			policyNamespacedName, err := getPolicyNamespacedName(db, deleteCompliance.PolicyID)
			if err != nil || policyNamespacedName == "" {
				// the policy may deleted from the db
				log.Debug("failed to get policy namespaced name - %v: %w", deleteCompliance.PolicyID, err)
				continue
			}
			if resp, err := requester.GetHttpClient().K8SPolicyIsPropagatedToK8SClusterServiceHTTPClient.
				DeleteK8SPolicyIsPropagatedToK8SCluster(ctx, deleteK8SPolicyIsPropagatedToK8SCluster(
					policyNamespacedName, deleteCompliance.ClusterName, leafHub, mchVersion)); err != nil && !errors.IsNotFound(err) {
				log.Errorf("failed to delete k8s policy is propagated to k8s cluster -%v: %w", resp, err)
			}
		}
	}

	return nil
}

func updateK8SPolicyIsPropagatedToK8SCluster(subjectId, objectId, status, reporterInstanceId string, mchVersion string) *kesselv1betarelations.
	UpdateK8SPolicyIsPropagatedToK8SClusterRequest {
	var relationStatus kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_Status
	switch status {
	case "non_compliant":
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_VIOLATIONS
	case "compliant":
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_NO_VIOLATIONS
	default:
		relationStatus = kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail_STATUS_OTHER
	}
	return &kesselv1betarelations.UpdateK8SPolicyIsPropagatedToK8SClusterRequest{
		K8SpolicyIspropagatedtoK8Scluster: &kesselv1betarelations.K8SPolicyIsPropagatedToK8SCluster{
			Metadata: &kesselv1betarelations.Metadata{
				RelationshipType: "k8spolicy_ispropagatedto_k8scluster",
			},
			ReporterData: &kesselv1betarelations.ReporterData{
				ReporterType:           kesselv1betarelations.ReporterData_ACM,
				ReporterInstanceId:     reporterInstanceId,
				ReporterVersion:        mchVersion,
				SubjectLocalResourceId: subjectId,
				ObjectLocalResourceId:  objectId,
			},
			RelationshipData: &kesselv1betarelations.K8SPolicyIsPropagatedToK8SClusterDetail{
				Status: relationStatus,
			},
		},
	}
}

func deleteK8SPolicyIsPropagatedToK8SCluster(subjectId, objectId, reporterInstanceId string, mchVersion string) *kesselv1betarelations.
	DeleteK8SPolicyIsPropagatedToK8SClusterRequest {
	return &kesselv1betarelations.DeleteK8SPolicyIsPropagatedToK8SClusterRequest{
		ReporterData: &kesselv1betarelations.ReporterData{
			ReporterType:           kesselv1betarelations.ReporterData_ACM,
			ReporterInstanceId:     reporterInstanceId,
			ReporterVersion:        mchVersion,
			SubjectLocalResourceId: subjectId,
			ObjectLocalResourceId:  objectId,
		},
	}
}

// generateCreateUpdateCompliances compare the
// generates the create/update/delete compliances which should post to db and inventory
func generateCreateUpdateDeleteCompliances(
	localCompliancesFromEvents []models.LocalStatusCompliance,
	complianceFromDb map[database.ComplianceStatus]set.Set,
	existClustersOnDB set.Set,
	leafHub string,
	policyID string,
) (
	[]models.LocalStatusCompliance,
	[]models.LocalStatusCompliance,
	[]models.LocalStatusCompliance,
) {
	clusterToComplianceMap := make(map[string]database.ComplianceStatus)
	for _, compliance := range localCompliancesFromEvents {
		clusterToComplianceMap[compliance.ClusterName] = compliance.Compliance
	}

	clusterToComplianceInDbMap := make(map[string]database.ComplianceStatus)
	for status, clusters := range complianceFromDb {
		for _, cluster := range clusters.ToSlice() {
			clusterToComplianceInDbMap[cluster.(string)] = status
		}
	}

	var createCompliances []models.LocalStatusCompliance
	var updateCompliances []models.LocalStatusCompliance
	var deleteCompliances []models.LocalStatusCompliance

	for cluster, compliance := range clusterToComplianceMap {
		if _, ok := clusterToComplianceInDbMap[cluster]; !ok {
			createCompliances = append(createCompliances, models.LocalStatusCompliance{
				LeafHubName: leafHub,
				PolicyID:    policyID,
				ClusterName: cluster,
				Error:       database.ErrorNone,
				Compliance:  compliance,
			})
		} else if clusterToComplianceInDbMap[cluster] != compliance {
			updateCompliances = append(updateCompliances, models.LocalStatusCompliance{
				LeafHubName: leafHub,
				PolicyID:    policyID,
				ClusterName: cluster,
				Error:       database.ErrorNone,
				Compliance:  compliance,
			})
		}
	}
	if existClustersOnDB == nil {
		return createCompliances, updateCompliances, deleteCompliances
	}

	for _, name := range existClustersOnDB.ToSlice() {
		clusterName, ok := name.(string)
		if !ok {
			continue
		}
		deleteCompliances = append(deleteCompliances, models.LocalStatusCompliance{
			LeafHubName: leafHub,
			PolicyID:    policyID,
			ClusterName: clusterName,
		})
	}
	return createCompliances, updateCompliances, deleteCompliances
}

func newLocalCompliances(leafHub, policyID string, compliance database.ComplianceStatus,
	eventComplianceClusters []string, allClustersOnDB set.Set,
) []models.LocalStatusCompliance {
	compliances := make([]models.LocalStatusCompliance, 0)
	for _, cluster := range eventComplianceClusters {
		compliances = append(compliances, models.LocalStatusCompliance{
			LeafHubName: leafHub,
			PolicyID:    policyID,
			ClusterName: cluster,
			Error:       database.ErrorNone,
			Compliance:  compliance,
		})
		allClustersOnDB.Remove(cluster)
	}
	return compliances
}

func getLocalComplianceClusterSets(db *gorm.DB, query interface{}, args ...interface{}) (
	map[string]*PolicyClustersSets, error,
) {
	var compliancesFromDB []models.LocalStatusCompliance
	err := db.Where(query, args...).Find(&compliancesFromDB).Error
	if err != nil {
		return nil, err
	}

	// policyID: { compliance: (cluster1, cluster2), nonCompliance: (cluster3, cluster4), unknowns: (cluster5) }
	allPolicyComplianceRowsFromDB := make(map[string]*PolicyClustersSets)
	for _, compliance := range compliancesFromDB {
		if _, ok := allPolicyComplianceRowsFromDB[compliance.PolicyID]; !ok {
			allPolicyComplianceRowsFromDB[compliance.PolicyID] = NewPolicyClusterSets()
		}
		allPolicyComplianceRowsFromDB[compliance.PolicyID].AddCluster(
			compliance.ClusterName, compliance.Compliance)
	}
	return allPolicyComplianceRowsFromDB, nil
}

func getPolicyNamespacedName(db *gorm.DB, policyID string) (string, error) {
	var policyFromDB []models.LocalSpecPolicy
	var resourceVersions []models.ResourceVersion
	if db == nil {
		return "", fmt.Errorf("db is nil")
	}
	err := db.Select(
		"policy_id AS key, concat(payload->'metadata'->>'namespace', '/', payload->'metadata'->>'name') AS name").
		Where(&models.LocalSpecPolicy{
			PolicyID: policyID,
		}).Find(&policyFromDB).Scan(&resourceVersions).Error
	if err != nil {
		return "", err
	}
	if len(resourceVersions) == 0 {
		return "", fmt.Errorf("policy %s not found", policyID)
	}
	return resourceVersions[0].Name, nil
}
