package policy

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/api/errors"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var log = logger.DefaultZapLogger()

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
	log.Debugw(startMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)

	var bundle generic.GenericBundle[policiesv1.Policy]
	err := evt.DataAs(&bundle)
	if err != nil {
		return fmt.Errorf("failed to unmarshal bundle - %v", err)
	}

	if err := h.insertOrUpdate(bundle.Resync, leafHubName); err != nil {
		return fmt.Errorf("failed to resync local policies - %w", err)
	}

	if err := h.insertOrUpdate(bundle.Create, leafHubName); err != nil {
		return fmt.Errorf("failed to insert local policies - %w", err)
	}

	if err := h.insertOrUpdate(bundle.Update, leafHubName); err != nil {
		return fmt.Errorf("failed to update local policies - %w", err)
	}

	db := database.GetGorm()
	if len(bundle.Delete) > 0 {
		for _, deleted := range bundle.Delete {
			if deleted.ID != "" {
				err = db.Where("leaf_hub_name", leafHubName).Where("policy_id", deleted.ID).
					Delete(&models.LocalSpecPolicy{}).Error
				if err != nil {
					return fmt.Errorf("failed deleting local policy - %v", err)
				}
			} else if deleted.Name != "" && deleted.Namespace != "" {
				err = db.Model(&models.LocalSpecPolicy{}).
					Where("leaf_hub_name = ?", leafHubName).
					Where("(payload -> 'metadata' ->> 'name') = ?", deleted.Name).
					Where("(payload -> 'metadata' ->> 'namespace') = ?", deleted.Namespace).
					Delete(&models.LocalSpecPolicy{}).Error
				if err != nil {
					return fmt.Errorf("failed to deleting local policy by namesapce/name - %v", err)
				}
			} else {
				log.Warnw("local policy delete event without ID or Name/Namespace", "LH", leafHubName)
			}
		}
	}

	// delete local policies that are not in the bundle.
	if len(bundle.ResyncMetadata) > 0 {
		var ids []string
		err = db.Model(&models.LocalSpecPolicy{}).Where("leaf_hub_name = ?", leafHubName).Pluck("policy_id", &ids).Error
		if err != nil {
			return fmt.Errorf("failed to get existing policy IDs - %w", err)
		}

		deletingIds := []string{}
		for _, id := range ids {
			metadata := bundle.FoundMetadataById(id)
			if metadata == nil {
				deletingIds = append(deletingIds, id)
			}
		}

		if len(deletingIds) == 0 {
			log.Debugw("no local policies to delete", "LH", leafHubName)
		} else {
			err = db.Transaction(func(tx *gorm.DB) error {
				for _, id := range deletingIds {
					// https://gorm.io/docs/delete.html#Soft-Delete
					if err = tx.Where(&models.LocalSpecPolicy{
						PolicyID:    id,
						LeafHubName: leafHubName,
					}).Delete(&models.LocalSpecPolicy{}).Error; err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				return fmt.Errorf("failed deleting local policies - %w", err)
			}
			log.Debugw("deleted local policies", "LH", leafHubName, "count", len(deletingIds))
		}
	}

	if configs.IsInventoryAPIEnabled() {
		err = h.postPolicyToInventoryApi(ctx, db, bundle, leafHubName)
		if err != nil {
			return fmt.Errorf("failed syncing inventory - %w", err)
		}
	}
	log.Debugw(finishMessage, "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)
	return nil
}

func (h *localPolicySpecHandler) insertOrUpdate(objs []policiesv1.Policy, leafHubName string) error {
	db := database.GetGorm()
	batchLocalPolicySpec := []models.LocalSpecPolicy{}
	for _, obj := range objs {
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

	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchLocalPolicySpec, 100).Error
	if err != nil {
		return err
	}
	return nil
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
	bundle generic.GenericBundle[policiesv1.Policy],
	leafHubName string,
) error {
	clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	log.Debugf("clusterInfo: %v", clusterInfo)
	if err != nil || clusterInfo.MchVersion == "" {
		log.Errorf("failed to get cluster info from db - %v", err)
	}

	if len(bundle.Create) > 0 {
		for _, policy := range bundle.Create {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, clusterInfo.MchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.CreateK8SPolicy(
				ctx, &kessel.CreateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil && !errors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create k8sCluster %v: %w", resp, err)
			}
		}
	}
	if len(bundle.Update) > 0 {
		for _, policy := range bundle.Update {
			k8sPolicy := generateK8SPolicy(&policy, leafHubName, clusterInfo.MchVersion)
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.UpdateK8SPolicy(
				ctx, &kessel.UpdateK8SPolicyRequest{K8SPolicy: k8sPolicy}); err != nil {
				return fmt.Errorf("failed to update k8sCluster %v: %w", resp, err)
			}
		}
	}
	if len(bundle.Delete) > 0 {
		for _, policy := range bundle.Delete {
			if resp, err := h.requester.GetHttpClient().PolicyServiceClient.DeleteK8SPolicy(
				ctx, &kessel.DeleteK8SPolicyRequest{
					ReporterData: &kessel.ReporterData{
						ReporterType:       kessel.ReporterData_ACM,
						ReporterInstanceId: leafHubName,
						ReporterVersion:    clusterInfo.MchVersion,
						LocalResourceId:    policy.Name,
					},
				}); err != nil && !errors.IsNotFound(err) {
				return fmt.Errorf("failed to delete k8sCluster %v: %w", resp, err)
			}
			// delete the policy related compliance data in inventory when policy is deleted
			err := h.deleteAllComplianceDataOfPolicy(ctx, db, h.requester, leafHubName, &policy, clusterInfo.MchVersion)
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
	policyMetadata *generic.ObjectMetadata,
	mchVersion string,
) error {
	if requester == nil {
		return fmt.Errorf("requester is nil")
	}
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	deleteCompliances, err := getComplianceDataOfPolicy(db, leafHubName, policyMetadata.ID)
	if err != nil {
		return err
	}
	if len(deleteCompliances) == 0 {
		return nil
	}
	return postCompliancesToInventoryApi(policyMetadata.Name, requester, leafHubName,
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
