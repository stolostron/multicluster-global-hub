package managedcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-kratos/kratos/v2/log"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/stolostron/multicloud-operators-foundation/pkg/klusterlet/clusterclaim"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/handlers/managedhub"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const BatchSize = 50

type managedClusterHandler struct {
	log           *zap.SugaredLogger
	eventType     string
	eventSyncMode enum.EventSyncMode
	eventPriority conflator.ConflationPriority
	requester     transport.Requester
}

func RegisterManagedClusterHandler(c client.Client, conflationManager *conflator.ConflationManager) {
	eventType := string(enum.ManagedClusterType)
	logName := strings.Replace(eventType, enum.EventTypePrefix, "", -1)
	h := &managedClusterHandler{
		log:           logger.ZapLogger(logName),
		eventType:     eventType,
		eventSyncMode: enum.CompleteStateMode,
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
	h.log.Debugw("handler start", "type", evt.Type(), "LH", evt.Source(), "version", version)

	var data []clusterv1.ManagedCluster
	if err := evt.DataAs(&data); err != nil {
		return err
	}

	db := database.GetGorm()
	clusterIdToVersionMapFromDB, err := getClusterIdToVersionMap(db, leafHubName)
	if err != nil {
		return fmt.Errorf("failed fetching leaf hub managed clusters from db - %w", err)
	}
	copyclusterIdToVersionMapFromDB := maps.Clone(clusterIdToVersionMapFromDB)

	// batch update/insert managed clusters
	batchManagedClusters := []models.ManagedCluster{}
	for _, object := range data {
		cluster := object

		// Initially, if the clusterID is not exist we will skip it until we get it from ClusterClaim
		clusterId := ""
		for _, claim := range cluster.Status.ClusterClaims {
			if claim.Name == "id.k8s.io" {
				clusterId = claim.Value
				break
			}
		}
		if clusterId == "" {
			continue
		}

		payload, err := json.Marshal(cluster)
		if err != nil {
			return err
		}

		clusterVersionFromDB, exist := clusterIdToVersionMapFromDB[clusterId]
		if !exist {
			batchManagedClusters = append(batchManagedClusters, models.ManagedCluster{
				ClusterID:   clusterId,
				LeafHubName: leafHubName,
				Payload:     payload,
				Error:       database.ErrorNone,
			})
			continue
		}

		// remove the handled object from the map
		delete(clusterIdToVersionMapFromDB, clusterId)

		if cluster.GetResourceVersion() == clusterVersionFromDB.ResourceVersion {
			continue // update cluster in db only if what we got is a different (newer) version of the resource
		}

		batchManagedClusters = append(batchManagedClusters, models.ManagedCluster{
			ClusterID:   clusterId,
			LeafHubName: leafHubName,
			Payload:     payload,
			Error:       database.ErrorNone,
		})
	}
	err = db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).CreateInBatches(batchManagedClusters, BatchSize).Error
	if err != nil {
		return err
	}

	// delete objects that in the db but were not sent in the bundle (leaf hub sends only living resources).
	// https://gorm.io/docs/delete.html#Soft-Delete
	err = db.Transaction(func(tx *gorm.DB) error {
		for clusterId := range clusterIdToVersionMapFromDB {
			e := tx.Where(&models.ManagedCluster{
				LeafHubName: leafHubName,
				ClusterID:   clusterId,
			}).Delete(&models.ManagedCluster{}).Error
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed deleting managed clusters - %w", err)
	}
	if configs.IsInventoryAPIEnabled() {
		err = h.syncInventory(
			ctx,
			data,
			leafHubName,
			copyclusterIdToVersionMapFromDB,
		)
		if err != nil {
			return fmt.Errorf("failed syncing inventory - %w", err)
		}
	}
	h.log.Debugw("handler finished", "type", evt.Type(), "LH", evt.Source(), "version", version)
	return nil
}

func (h *managedClusterHandler) syncInventory(
	ctx context.Context,
	data []clusterv1.ManagedCluster,
	leafHubName string,
	clusterIdToVersionMapFromDB map[string]models.ResourceVersion,
) error {
	createClusters, updateClusters, deleteClusters := h.generateCreateUpdateDeleteClusters(
		data,
		leafHubName,
		clusterIdToVersionMapFromDB,
	)
	if len(createClusters) == 0 && len(updateClusters) == 0 && len(deleteClusters) == 0 {
		h.log.Debugw("no changes to managed clusters", "LH", leafHubName)
		return nil
	}
	if h.requester == nil {
		return fmt.Errorf("requester is nil")
	}

	clusterInfo, err := managedhub.GetClusterInfo(database.GetGorm(), leafHubName)
	log.Debugf("clusterInfo: %v", clusterInfo)
	if err != nil {
		h.log.Warnf("failed to get cluster info from db - %w", err)
	}
	h.postToInventoryApi(
		ctx,
		h.requester,
		clusterInfo,
		createClusters,
		updateClusters,
		deleteClusters,
		leafHubName,
	)
	return nil
}

func (h *managedClusterHandler) postToInventoryApi(
	ctx context.Context,
	requester transport.Requester,
	clusterInfo models.ClusterInfo,
	createClusters,
	updateClusters []clusterv1.ManagedCluster,
	deleteClusters []models.ResourceVersion,
	leafHubName string,
) {
	if len(createClusters) > 0 {
		for _, cluster := range createClusters {
			k8sCluster := GetK8SCluster(ctx, &cluster, leafHubName, clusterInfo)
			if resp, err := requester.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
				&kessel.CreateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
				h.log.Errorf("failed to create k8sCluster %v: %w", resp, err)
			}
		}
	}

	if len(updateClusters) > 0 {
		for _, cluster := range updateClusters {
			k8sCluster := GetK8SCluster(context.TODO(), &cluster, leafHubName, clusterInfo)
			if resp, err := requester.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
				&kessel.UpdateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
				h.log.Errorf("failed to update k8sCluster %v: %w", resp, err)
			}
		}
	}

	if len(deleteClusters) > 0 {
		for _, cluster := range deleteClusters {
			if resp, err := requester.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
				&kessel.DeleteK8SClusterRequest{ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: leafHubName,
					LocalResourceId:    cluster.Name,
				}}); err != nil {
				h.log.Errorf("failed to delete k8sCluster %v: %w", resp, err)
			}
		}
	}
}

// generateCreateUpdateDeleteClusters generates the create, update and delete clusters
// from the managed clusters in the database and the managed clusters in the bundle.
func (h *managedClusterHandler) generateCreateUpdateDeleteClusters(
	data []clusterv1.ManagedCluster,
	leafHubName string,
	// clusterIdToVersionMapFromDB is a map of clusterID to resourceVersion
	// for all managed clusters in the database
	clusterIdToVersionMapFromDB map[string]models.ResourceVersion) (
	[]clusterv1.ManagedCluster,
	[]clusterv1.ManagedCluster,
	[]models.ResourceVersion,
) {
	var createClusters []clusterv1.ManagedCluster
	var updateClusters []clusterv1.ManagedCluster
	var deleteClusters []models.ResourceVersion

	for _, object := range data {
		cluster := object
		// Initially, if the clusterID is not exist we will skip it until we get it from ClusterClaim
		clusterId := ""
		for _, claim := range cluster.Status.ClusterClaims {
			if claim.Name == constants.ClusterIdClaimName {
				clusterId = claim.Value
				break
			}
		}
		if clusterId == "" {
			continue
		}
		// create clusters
		clusterVersionFromDB, exist := clusterIdToVersionMapFromDB[clusterId]
		if !exist {
			createClusters = append(createClusters, cluster)
			continue
		}

		// remove the handled object from the map
		delete(clusterIdToVersionMapFromDB, clusterId)

		if cluster.GetResourceVersion() == clusterVersionFromDB.ResourceVersion {
			continue // update cluster in db only if what we got is a different (newer) version of the resource
		}

		// update clusters
		updateClusters = append(updateClusters, cluster)
	}
	for _, rv := range clusterIdToVersionMapFromDB {
		deleteClusters = append(deleteClusters, rv)
	}
	return createClusters, updateClusters, deleteClusters
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

	// TODO: remove this after the vendorVersion is optional
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

	// TODO: should get nodelist from clusterInfo.Status.NodeList
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

func getClusterIdToVersionMap(db *gorm.DB, leafHubName string) (map[string]models.ResourceVersion, error) {
	var resourceVersions []models.ResourceVersion
	err := db.Select(
		"cluster_id AS key, cluster_name AS name, payload->'metadata'->>'resourceVersion' AS resource_version").
		Where(&models.ManagedCluster{
			LeafHubName: leafHubName,
		}).Find(&models.ManagedCluster{}).Scan(&resourceVersions).Error
	if err != nil {
		return nil, err
	}
	nameToVersionMap := make(map[string]models.ResourceVersion)
	for _, resource := range resourceVersions {
		nameToVersionMap[resource.Key] = resource
	}
	return nameToVersionMap, nil
}
