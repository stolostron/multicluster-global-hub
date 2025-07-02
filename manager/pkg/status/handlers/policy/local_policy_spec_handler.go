package policy

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-kratos/kratos/v2/log"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	startMessage  = "handler start"
	finishMessage = "handler finished"
)

type localPolicySpecHandler struct {
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	requester     transport.Requester
}

func RegisterLocalPolicySpecHandler(conflationManager *conflator.ConflationManager) {
	eventType := string(enum.LocalPolicySpecType)
	h := &localPolicySpecHandler{
		eventType:     eventType,
		eventSyncMode: enum.HybridStateMode,
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
func (h *localPolicySpecHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	log.Debugw(startMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)

	var bundle generic.GenericBundle
	if err := evt.DataAs(&bundle); err != nil {
		return err
	}

	batchLocalPolicySpec := []models.LocalSpecPolicy{}

	// update or create managed clusters in the database.
	for _, obj := range append(append(bundle.Create, bundle.Update...), bundle.Resync...) {
		payload, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		batchLocalPolicySpec = append(batchLocalPolicySpec, models.LocalSpecPolicy{
			PolicyID:    string(obj.GetUID()),
			LeafHubName: leafHubName,
			Payload:     payload,
		})
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchLocalPolicySpec, 100).Error
	if err != nil {
		return err
	}

	// delete managed clusters that are not in the bundle.
	var ids []string
	err = db.Model(&models.LocalSpecPolicy{}).Where("leaf_hub_name = ?", leafHubName).Pluck("policy_id", &ids).Error
	if err != nil {
		return fmt.Errorf("failed to get existing policy IDs - %w", err)
	}

	deletingObjects := []unstructured.Unstructured{}
	for _, object := range bundle.ResyncMetadata {
		if utils.ContainsString(ids, string(object.GetUID())) {
			continue
		}
		deletingObjects = append(deletingObjects, object)
	}

	// https://gorm.io/docs/delete.html#Soft-Delete
	if len(deletingObjects) == 0 {
		log.Debugw("no managed clusters to delete", "LH", leafHubName)
	} else {
		err = db.Transaction(func(tx *gorm.DB) error {
			for _, object := range deletingObjects {
				err = tx.Where(&models.LocalSpecPolicy{
					PolicyID:    string(object.GetUID()),
					LeafHubName: leafHubName,
				}).Delete(&models.LocalSpecPolicy{}).Error
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("failed deleting local policies - %w", err)
		}
		log.Debugw("deleted policies", "LH", leafHubName, "count", len(deletingObjects))
	}

	// if configs.IsInventoryAPIEnabled() {
	// 	err = h.syncInventory(ctx, db, data, leafHubName, copyPolicyIdToVersionMapFromDB)
	// 	if err != nil {
	// 		return fmt.Errorf("failed syncing inventory - %w", err)
	// 	}
	// }
	log.Debugw(finishMessage, "type", evt.Type(), "LH", evt.Source(), "version", version)
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
		data,
		leafHubName,
		policyIdToVersionMapFromDB,
	)
	if len(createPolicy) == 0 && len(updatePolicy) == 0 && len(deletePolicy) == 0 {
		log.Debugf("no policy to post to inventory")
		return nil
	}
	if h.requester == nil {
		return fmt.Errorf("requester is nil")
	}
	clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	log.Debugf("clusterInfo: %v", clusterInfo)
	if err != nil || clusterInfo.MchVersion == "" {
		log.Errorf("failed to get cluster info from db - %v", err)
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
				log.Errorf("failed to delete compliance data of policy %s: %v", policy.Name, err)
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
	deleteCompliances, err := getComplianceDataOfPolicy(db, leafHubName, policy.Key)
	if err != nil {
		return err
	}
	if len(deleteCompliances) == 0 {
		return nil
	}
	return postCompliancesToInventoryApi(policy.Name, requester, leafHubName,
		nil, nil, deleteCompliances, mchVersion)
}

func getComplianceDataOfPolicy(db *gorm.DB, leafHubName string, policyId string,
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
