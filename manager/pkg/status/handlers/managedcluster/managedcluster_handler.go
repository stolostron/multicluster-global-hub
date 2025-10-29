package managedcluster

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/stolostron/multicloud-operators-foundation/pkg/klusterlet/clusterclaim"
	"gorm.io/gorm/clause"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const BatchSize = 50

var log = logger.DefaultZapLogger()

type managedClusterHandler struct {
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	requester     transport.Requester
}

func RegisterManagedClusterHandler(c client.Client, conflationManager *conflator.ConflationManager) {
	eventType := string(enum.ManagedClusterType)
	h := &managedClusterHandler{
		eventType:     eventType,
		eventSyncMode: enum.HybridStateMode,
		eventPriority: conflator.ManagedClustersPriority,
		requester:     conflationManager.Requster,
	}
	conflationManager.Register(conflator.NewConflationRegistration(
		h.eventPriority,
		h.eventSyncMode,
		h.eventType,
		h.handleEvent,
	))
}

func (h *managedClusterHandler) handleEvent(ctx context.Context, evt *cloudevents.Event) error {
	version := evt.Extensions()[eventversion.ExtVersion]
	leafHubName := evt.Source()
	log.Debugw("handler start", "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)

	var bundle generic.GenericBundle[clusterv1.ManagedCluster]
	err := evt.DataAs(&bundle)
	if err != nil {
		log.Warnw("failed to unmarshal managed cluster bundle", "type", enum.ShortenEventType(evt.Type()),
			"LH", evt.Source(), "version", version, "error", err)
		return nil
	}

	// Handle insertOrUpdate operations for Resync, Create, and Update
	operations := [][]clusterv1.ManagedCluster{
		bundle.Resync,
		bundle.Create,
		bundle.Update,
	}

	for _, data := range operations {
		if err := h.insertOrUpdate(data, leafHubName); err != nil {
			return fmt.Errorf("failed to process managed clusters - %w", err)
		}
	}

	db := database.GetGorm()
	if len(bundle.Delete) > 0 {
		for _, deleted := range bundle.Delete {
			if deleted.ID != "" {
				err = db.Where("leaf_hub_name", leafHubName).Where("cluster_id", deleted.ID).
					Delete(&models.ManagedCluster{}).Error
				if err != nil {
					return fmt.Errorf("failed deleting managed clusters - %w", err)
				}
			} else if deleted.Name != "" {
				// if the cluster is deleted, we need to delete it by name and namespace
				err = db.Where("leaf_hub_name = ?", leafHubName).Where("cluster_name = ?", deleted.Name).
					Delete(&models.ManagedCluster{}).Error
				if err != nil {
					return fmt.Errorf("failed deleting managed clusters by name and namespace - %w", err)
				}
			} else {
				log.Warnw("managed cluster delete event without ID or Name/Namespace", "LH", leafHubName)
			}
		}
	}

	if len(bundle.ResyncMetadata) > 0 {
		// delete managed clusters that are not in the bundle.
		var ids []string
		err = db.Model(&models.ManagedCluster{}).Where("leaf_hub_name = ?", leafHubName).Pluck("cluster_id", &ids).Error
		if err != nil {
			return fmt.Errorf("failed to get existing cluster IDs - %w", err)
		}

		deletingIds := []string{}
		for _, id := range ids {
			// if the metadata is not found, it means the cluster is deleted
			metadata := bundle.FoundMetadataById(id)
			if metadata == nil {
				deletingIds = append(deletingIds, id)
			}
		}

		// https://gorm.io/docs/delete.html#Soft-Delete
		if len(deletingIds) == 0 {
			log.Debugw("no managed clusters to delete", "LH", leafHubName)
		} else {
			err = db.Where("leaf_hub_name", leafHubName).Where("cluster_id IN ?", deletingIds).
				Delete(&models.ManagedCluster{}).Error
			if err != nil {
				return fmt.Errorf("failed deleting managed clusters - %w", err)
			}
			log.Debugw("deleted managed clusters", "LH", leafHubName, "count", len(deletingIds))
		}
		return nil
	}

	if configs.IsInventoryAPIEnabled() {
		err = h.postToInventoryApi(
			ctx,
			h.requester,
			bundle,
			leafHubName,
		)
		if err != nil {
			return fmt.Errorf("failed syncing inventory - %w", err)
		}
	}
	log.Debugw("handler finished", "type", enum.ShortenEventType(evt.Type()), "LH", evt.Source(), "version", version)
	return nil
}

func (h *managedClusterHandler) postToInventoryApi(
	ctx context.Context,
	requester transport.Requester,
	bundle generic.GenericBundle[clusterv1.ManagedCluster],
	leafHubName string,
) error {
	leafHubClusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	log.Debugf("leafhub clusterInfo: %v", leafHubClusterInfo)
	if err != nil {
		log.Warnf("failed to get cluster info from db: %v", err)
	}

	if len(bundle.Create) > 0 {
		for _, cluster := range bundle.Create {
			k8sCluster := GetK8SCluster(ctx, &cluster, leafHubName, leafHubClusterInfo)
			if resp, err := requester.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
				&kessel.CreateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
				log.Errorf("failed to create k8sCluster %v: %v", resp, err)
				return err
			}
		}
	}

	if len(bundle.Update) > 0 {
		for _, cluster := range bundle.Update {
			k8sCluster := GetK8SCluster(context.TODO(), &cluster, leafHubName, leafHubClusterInfo)
			if resp, err := requester.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
				&kessel.UpdateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
				log.Errorf("failed to update k8sCluster %v: %v", resp, err)
				return err
			}
		}
	}

	if len(bundle.Delete) > 0 {
		for _, clusterMetadata := range bundle.Delete {
			if resp, err := requester.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
				&kessel.DeleteK8SClusterRequest{ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: leafHubName,
					LocalResourceId:    clusterMetadata.Name,
				}}); err != nil {
				log.Errorf("failed to delete k8sCluster %v: %v", resp, err)
				return err
			}
		}
	}
	return nil
}

func GetK8SCluster(ctx context.Context,
	cluster *clusterv1.ManagedCluster, leafHubName string,
	clusterInfo models.ClusterInfo,
) *kessel.K8SCluster {
	clusterId := string(cluster.GetUID())
	var vendorVersion, cloudVendor, kubeVersion, kubeVendor string
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == constants.ClusterIdClaimName {
			clusterId = claim.Value
		}
		if claim.Name == "platform.open-cluster-management.io" {
			cloudVendor = claim.Value
		}
		if claim.Name == "kubeversion.open-cluster-management.io" {
			kubeVersion = claim.Value
		}
		if claim.Name == "version.openshift.io" {
			vendorVersion = claim.Value
		}
		if claim.Name == "product.open-cluster-management.io" {
			kubeVendor = claim.Value
		}
	}

	if vendorVersion == "" {
		vendorVersion = kubeVersion
	}

	kesselLabels := []*kessel.ResourceLabel{}
	for key, value := range cluster.Labels {
		kesselLabels = append(kesselLabels, &kessel.ResourceLabel{
			Key:   key,
			Value: value,
		})
	}
	k8sCluster := &kessel.K8SCluster{
		Metadata: &kessel.Metadata{
			ResourceType: "k8s_cluster",
			Labels:       kesselLabels,
		},
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: leafHubName,
			LocalResourceId:    cluster.Name,
		},
		ResourceData: &kessel.K8SClusterDetail{
			ExternalClusterId: clusterId,
			KubeVersion:       kubeVersion,
			Nodes:             []*kessel.K8SClusterDetailNodesInner{},
		},
	}

	// platform
	switch cloudVendor {
	case clusterclaim.PlatformAWS:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	case clusterclaim.PlatformGCP:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_GCP_UPI
	case clusterclaim.PlatformBareMetal:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_BAREMETAL_UPI
	case clusterclaim.PlatformIBM:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_IBMCLOUD_UPI
	case clusterclaim.PlatformAzure:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	default:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_CLOUD_PLATFORM_OTHER
	}
	if clusterInfo.ConsoleURL != "" {
		k8sCluster.ReporterData.ConsoleHref = clusterInfo.ConsoleURL
	}
	if clusterInfo.MchVersion != "" {
		k8sCluster.ReporterData.ReporterVersion = clusterInfo.MchVersion
	}

	// kubevendor, ony have the openshift version
	switch kubeVendor {
	case clusterclaim.ProductOpenShift:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_OPENSHIFT
	case clusterclaim.ProductEKS:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_EKS
	case clusterclaim.ProductGKE:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_GKE
	default:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_KUBE_VENDOR_OTHER
	}
	k8sCluster.ResourceData.VendorVersion = vendorVersion

	// cluster status
	for _, cond := range cluster.Status.Conditions {
		if cond.Type == clusterv1.ManagedClusterConditionAvailable {
			if cond.Status == metav1.ConditionTrue {
				k8sCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_READY
			} else {
				k8sCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_FAILED
			}
		} else {
			k8sCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_OFFLINE
		}
	}

	kesselNode := &kessel.K8SClusterDetailNodesInner{
		Name: cluster.Name,
	}
	cpu, ok := cluster.Status.Capacity[clusterv1.ResourceCPU]
	if ok {
		kesselNode.Cpu = cpu.String()
	}
	memory, ok := cluster.Status.Capacity[clusterv1.ResourceMemory]
	if ok {
		kesselNode.Memory = memory.String()
	}
	k8sCluster.ResourceData.Nodes = []*kessel.K8SClusterDetailNodesInner{
		kesselNode,
	}
	return k8sCluster
}

func (h *managedClusterHandler) insertOrUpdate(objs []clusterv1.ManagedCluster, leafHubName string) error {
	if len(objs) == 0 {
		return nil
	}

	batchClusters := []models.ManagedCluster{}
	for _, obj := range objs {
		id := utils.GetClusterClaimID(&obj, "")
		if id == "" {
			log.Warnf("managed cluster %s has no cluster claim id, skip", obj.Name)
			continue
		}

		log.Debugf("inserting or updating cluster: name=%s, id=%s", obj.Name, id)

		payload, err := json.Marshal(obj)
		if err != nil {
			return fmt.Errorf("failed to marshal cluster %s: %w", obj.Name, err)
		}

		batchClusters = append(batchClusters, models.ManagedCluster{
			ClusterID:   id,
			LeafHubName: leafHubName,
			Payload:     payload,
			Error:       database.ErrorNone,
		})
	}

	db := database.GetGorm()
	err := db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchClusters, BatchSize).Error
	if err != nil {
		return fmt.Errorf("failed to insert or update clusters: %w", err)
	}
	return nil
}
