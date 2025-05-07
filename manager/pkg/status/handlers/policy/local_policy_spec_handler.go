package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/api/errors"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	startMessage  = "handler start"
	finishMessage = "handler finished"
)

type localPolicySpecHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	requester     transport.Requester
}

func RegisterLocalPolicySpecHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalPolicySpecType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &localPolicySpecHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
		eventPriority: conflator.LocalPolicySpecPriority,
		requester:     conflationManager.Requster,
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
	h.log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	db := database.GetGorm()
	policyIdToVersionMapFromDB, err := getPolicyIdToVersionMap(db, leafHubName)
	if err != nil {
		return err
	}
	copyPolicyIdToVersionMapFromDB := maps.Clone(policyIdToVersionMapFromDB)
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
		if specificObj.GetResourceVersion() == resourceVersionFromDB.ResourceVersion {
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
	if configs.IsInventoryAPIEnabled() {
		err = h.syncInventory(ctx, db, data, leafHubName, copyPolicyIdToVersionMapFromDB)
		if err != nil {
			return fmt.Errorf("failed syncing inventory - %w", err)
		}
	}
	h.log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func getPolicyIdToVersionMap(db *gorm.DB, leafHubName string) (map[string]models.ResourceVersion, error) {
	var resourceVersions []models.ResourceVersion
	err := db.Select(
		"policy_id AS key, concat(payload->'metadata'->>'namespace', '/', payload->'metadata'->>'name') AS name, payload->'metadata'->>'resourceVersion' AS resource_version").
		Where(&models.LocalSpecPolicy{ // Find soft deleted records: db.Unscoped().Where(...).Find(...)
			LeafHubName: leafHubName,
		}).
		Find(&models.LocalSpecPolicy{}).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}
	idToVersionMap := make(map[string]models.ResourceVersion)
	for _, resource := range resourceVersions {
		idToVersionMap[resource.Key] = resource
	}

	return idToVersionMap, nil
}

func (h *localPolicySpecHandler) syncInventory(
	ctx context.Context,
	db *gorm.DB,
	data []policiesv1.Policy,
	leafHubName string,
	policyIdToVersionMapFromDB map[string]models.ResourceVersion,
) error {
	createPolicy, updatePolicy, deletePolicy := generateCreateUpdateDeletePolicy(
		h.log,
		data,
		leafHubName,
		policyIdToVersionMapFromDB,
	)
	if len(createPolicy) == 0 && len(updatePolicy) == 0 && len(deletePolicy) == 0 {
		h.log.Debugf("no policy to post to inventory")
		return nil
	}
	if h.requester == nil {
		return fmt.Errorf("requester is nil")
	}
	clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	h.log.Debugf("clusterInfo: %v", clusterInfo)
	if err != nil || clusterInfo.MchVersion == "" {
		h.log.Errorf("failed to get cluster info from db - %v", err)
	}
	return h.postPolicyToInventoryApi(
		ctx,
		db,
		createPolicy,
		updatePolicy,
		deletePolicy,
		leafHubName,
		clusterInfo.MchVersion,
	)
}

// generateCreateUpdateDeletePolicy generates the create, update and delete Policy
// from the Policy in the database and the Policy in the bundle.
func generateCreateUpdateDeletePolicy(
	log *zap.SugaredLogger,
	data []policiesv1.Policy,
	leafHubName string,
	policyIdToVersionMapFromDB map[string]models.ResourceVersion) (
	[]policiesv1.Policy,
	[]policiesv1.Policy,
	[]models.ResourceVersion,
) {
	var createPolicy []policiesv1.Policy
	var updatePolicy []policiesv1.Policy
	var deletePolicy []models.ResourceVersion

	for _, object := range data {
		specificObj := object
		log.Debugf("policy: %v", specificObj)
		uid := string(specificObj.GetUID())
		log.Debugf("policy uid: %s", uid)
		if uid == "" {
			continue
		}
		resourceVersionFromDB, objInDB := policyIdToVersionMapFromDB[uid]

		// if the row doesn't exist in db then add it.
		if !objInDB {
			createPolicy = append(createPolicy, specificObj)
			continue
		}
		// remove the existing object from the map
		delete(policyIdToVersionMapFromDB, uid)

		// update object only if what we got is a different (newer) version of the resource
		if specificObj.GetResourceVersion() == resourceVersionFromDB.ResourceVersion {
			continue
		}
		updatePolicy = append(updatePolicy, specificObj)
	}
	for _, rv := range policyIdToVersionMapFromDB {
		deletePolicy = append(deletePolicy, rv)
	}
	return createPolicy, updatePolicy, deletePolicy
}

func generateK8SPolicy(policy *policiesv1.Policy, reporterInstanceId string, mchVersion string) *kessel.K8SPolicy {
	kesselLabels := []*kessel.ResourceLabel{}
	for key, value := range policy.Labels {
		kesselLabels = append(kesselLabels, &kessel.ResourceLabel{
			Key:   key,
			Value: value,
		})
	}
	for key, value := range policy.Annotations {
		kesselLabels = append(kesselLabels, &kessel.ResourceLabel{
			Key:   key,
			Value: value,
		})
	}
	return &kessel.K8SPolicy{
		Metadata: &kessel.Metadata{
			ResourceType: "k8s_policy",
			Labels:       kesselLabels,
		},
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: reporterInstanceId,
			ReporterVersion:    mchVersion,
			LocalResourceId:    policy.Namespace + "/" + policy.Name,
		},
		ResourceData: &kessel.K8SPolicyDetail{
			Disabled: policy.Spec.Disabled,
			Severity: kessel.K8SPolicyDetail_MEDIUM, // need to update
		},
	}
}

// postPolicyToInventoryApi posts the policy to inventory api
func (h *localPolicySpecHandler) postPolicyToInventoryApi(
	ctx context.Context,
	db *gorm.DB,
	createPolicy []policiesv1.Policy,
	updatePolicy []policiesv1.Policy,
	deletePolicy []models.ResourceVersion,
	leafHubName string,
	mchVersion string,
) error {
	if len(createPolicy) > 0 {
		for _, policy := range createPolicy {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, mchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(
				ctx, &kessel.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create k8sCluster %v: %w", resp, err)
			}
		}
	}
	if len(updatePolicy) > 0 {
		for _, policy := range updatePolicy {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, mchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(
				ctx, &kessel.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil {
				return fmt.Errorf("failed to update k8sCluster %v: %w", resp, err)
			}
		}
	}
	if len(deletePolicy) > 0 {
		for _, policy := range deletePolicy {
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(
				ctx, &kessel.DeleteK8SPolicyRequest{
					ReporterData: &kessel.ReporterData{
						ReporterType:       kessel.ReporterData_ACM,
						ReporterInstanceId: leafHubName,
						ReporterVersion:    mchVersion,
						LocalResourceId:    policy.Name,
					},
				}); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete k8sCluster %v: %w", resp, err)
			}
			// delete the policy related compliance data in inventory when policy is deleted
			err := h.deleteAllComplianceDataOfPolicy(ctx, db, h.requester, leafHubName, policy, mchVersion)
			if err != nil {
				h.log.Errorf("failed to delete compliance data of policy %s: %w", policy.Name, err)
			}
		}
	}
	return nil
}

func (h *localPolicySpecHandler) deleteAllComplianceDataOfPolicy(
	ctx context.Context,
	db *gorm.DB,
	requester transport.Requester,
	leafHubName string,
	policy models.ResourceVersion,
	mchVersion string,
) error {
	if requester == nil {
		return fmt.Errorf("requester is nil")
	}
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	deleteCompliances, err := getComplianceDataOfPolicy(ctx, db, leafHubName, policy.Key)
	if err != nil {
		return err
	}
	if len(deleteCompliances) == 0 {
		return nil
	}
	return postCompliancesToInventoryApi(ctx, db, h.log, requester, leafHubName, nil, nil, deleteCompliances, mchVersion)
}

func getComplianceDataOfPolicy(ctx context.Context, db *gorm.DB, leafHubName string, policyId string,
) ([]models.LocalStatusCompliance, error) {
	var complianceData []models.LocalStatusCompliance
	err := db.Select("policy_id, cluster_name, leaf_hub_name").
		Where(&models.LocalStatusCompliance{ // Find soft deleted records: db.Unscoped().Where(...).Find(...)
			LeafHubName: leafHubName,
			PolicyID:    policyId,
		}).Find(&models.LocalStatusCompliance{}).Scan(&complianceData).Error
	if err != nil {
		return nil, err
	}
	return complianceData, nil
}
