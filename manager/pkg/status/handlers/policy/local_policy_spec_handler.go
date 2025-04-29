package policy

import (
	"context"
	"encoding/json"
	"fmt"
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
	h.log.Debugf("policyIdToVersionMapFromDB: %v", policyIdToVersionMapFromDB)
	if err != nil {
		return err
	}

	var data []policiesv1.Policy
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	createPolicy, updatePolicy, deletePolicy := generateCreateUpdateDeletePolicy(
		h.log,
		data,
		leafHubName,
		policyIdToVersionMapFromDB,
	)
	h.log.Debugw("create, update, delete policy",
		"LH", leafHubName,
		"create", createPolicy,
		"update", updatePolicy,
		"delete", deletePolicy,
	)
	if len(createPolicy) == 0 && len(updatePolicy) == 0 && len(deletePolicy) == 0 {
		h.log.Debugw("no changes to managed Policy", "LH", leafHubName)
		return nil
	}

	// The data in inventory should be same as the data in the database
	// so we will post to inventory if there is some changes based on the data in database
	// Note: we do not handle the case: when run globalhub sometimes, and the data has post
	// database then enable inventory api
	if configs.IsInventoryAPIEnabled() {
		if h.requester == nil {
			return fmt.Errorf("requester is nil")
		}
		clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
		h.log.Debugf("clusterInfo: %v", clusterInfo)
		if err != nil || clusterInfo.MchVersion == "" {
			h.log.Errorf("failed to get cluster info from db - %v", err)
		}

		createClustersToInventory, updateClustersToInventory, deleteClustersFromInventory := h.postPolicyToInventoryApi(
			ctx,
			createPolicy,
			updatePolicy,
			deletePolicy,
			leafHubName,
			clusterInfo.MchVersion,
		)
		h.log.Debugw("post to inventory api", "LH",
			leafHubName, "create",
			createClustersToInventory, "update",
			updateClustersToInventory, "delete",
			deleteClustersFromInventory)

		// we only post the data to database which successfully posted to inventory
		createPolicy = createClustersToInventory
		updatePolicy = updateClustersToInventory
		deletePolicy = deleteClustersFromInventory
	}

	err = h.postPolicyToDatabase(
		ctx,
		db,
		leafHubName,
		createPolicy,
		updatePolicy,
		deletePolicy,
	)
	if err != nil {
		return fmt.Errorf("failed posting managed clusters to database - %w", err)
	}
	h.log.Debugw("post to database",
		"LH", leafHubName, "create",
		createPolicy, "update",
		updatePolicy, "delete",
		deletePolicy)
	return nil
}

func (h *localPolicySpecHandler) postPolicyToDatabase(
	ctx context.Context,
	db *gorm.DB,
	leafHubName string,
	createPolicy []policiesv1.Policy,
	updatePolicy []policiesv1.Policy,
	deletePolicy []models.ResourceVersion,
) error {
	batchLocalPolicySpec := []models.LocalSpecPolicy{}
	updatePolicy = append(updatePolicy, createPolicy...)
	for _, object := range updatePolicy {
		policy := object
		h.log.Debugf("policy: %v", policy)
		payload, err := json.Marshal(policy)
		if err != nil {
			return err
		}

		batchLocalPolicySpec = append(batchLocalPolicySpec, models.LocalSpecPolicy{
			PolicyID:    string(policy.GetUID()),
			LeafHubName: leafHubName,
			Payload:     payload,
		})
	}
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchLocalPolicySpec, 100).Error
	if err != nil {
		return err
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	// https://gorm.io/docs/transactions.html
	// https://gorm.io/docs/delete.html#Soft-Delete
	var deletedFromDb []models.LocalSpecPolicy
	for _, policy := range deletePolicy {
		deletedFromDb = append(deletedFromDb, models.LocalSpecPolicy{
			PolicyID: policy.Key,
		})
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, policy := range deletedFromDb {
			e := tx.Where(&policy).Delete(&models.LocalSpecPolicy{}).Error
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed deleting policy - %w", err)
	}
	return nil
}

// postPolicyToInventoryApi posts the policy to inventory api
func (h *localPolicySpecHandler) postPolicyToInventoryApi(
	ctx context.Context,
	createPolicy []policiesv1.Policy,
	updatePolicy []policiesv1.Policy,
	deletePolicy []models.ResourceVersion,
	leafHubName string,
	mchVersion string,
) (
	[]policiesv1.Policy,
	[]policiesv1.Policy,
	[]models.ResourceVersion,
) {
	var createPolicyToInventory []policiesv1.Policy
	var updatePolicyToInventory []policiesv1.Policy
	var deletePolicyInInventory []models.ResourceVersion

	if len(createPolicy) > 0 {
		for _, policy := range createPolicy {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, mchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(
				ctx, &kessel.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil && !errors.IsAlreadyExists(err) {
				h.log.Errorf("failed to create k8sCluster %v: %w", resp, err)
			} else {
				createPolicyToInventory = append(createPolicyToInventory, policy)
			}
		}
	}
	if len(updatePolicy) > 0 {
		for _, policy := range updatePolicy {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, mchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(
				ctx, &kessel.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil {
				h.log.Errorf("failed to update k8sCluster %v: %w", resp, err)
			} else {
				updatePolicyToInventory = append(updatePolicyToInventory, policy)
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
				}); err != nil {
				h.log.Errorf("failed to delete k8sCluster %v: %w", resp, err)
			} else {
				deletePolicyInInventory = append(deletePolicyInInventory, policy)
			}
		}
	}
	return createPolicyToInventory, updatePolicyToInventory, deletePolicyInInventory
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
