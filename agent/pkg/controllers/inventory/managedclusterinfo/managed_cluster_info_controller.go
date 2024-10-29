package managedclusterinfo

import (
	"context"
	"fmt"
	"reflect"

	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/requester"
)

type ManagedClusterInfoInventorySyncer struct {
	log           *zap.SugaredLogger
	runtimeClient client.Client
	requester     transport.Requester
	clientCN      string
}

func (r *ManagedClusterInfoInventorySyncer) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clusterInfo := &clusterinfov1beta1.ManagedClusterInfo{}
	err := r.runtimeClient.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, clusterInfo)
	if errors.IsNotFound(err) {
		r.log.Infof("clusterInfo(%s) not found. Ignoring since it must have been deleted.", req.Name)
		return ctrl.Result{}, nil
	} else if err != nil {
		return ctrl.Result{}, err
	}

	k8sCluster := GetK8SCluster(clusterInfo, r.clientCN)

	annotations := clusterInfo.GetAnnotations()
	if annotations != nil {
		if _, ok := annotations[constants.InventoryResourceCreatingAnnotationlKey]; ok {
			if resp, err := r.requester.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
				&kessel.CreateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create k8sCluster %v: %w", resp, err)
			}
			return ctrl.Result{}, nil
		}
	}

	if clusterInfo.DeletionTimestamp.IsZero() {
		// add a finalizer to the managedclusterinfo object
		if !controllerutil.ContainsFinalizer(clusterInfo, constants.InventoryResourceFinalizer) {
			controllerutil.AddFinalizer(clusterInfo, constants.InventoryResourceFinalizer)
			if err := r.runtimeClient.Update(ctx, clusterInfo); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The managedclusterinfo object is being deleted
		if controllerutil.ContainsFinalizer(clusterInfo, constants.InventoryResourceFinalizer) {
			if resp, err := r.requester.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
				&kessel.DeleteK8SClusterRequest{ReporterData: k8sCluster.ReporterData}); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to delete k8sCluster %v: %w", resp, err)
			}
			// remove finalizer
			controllerutil.RemoveFinalizer(clusterInfo, constants.InventoryResourceFinalizer)
			if err := r.runtimeClient.Update(ctx, clusterInfo); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	if resp, err := r.requester.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
		&kessel.UpdateK8SClusterRequest{K8SCluster: k8sCluster}); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update k8sCluster %v: %w", resp, err)
	}
	return ctrl.Result{}, nil
}

func AddManagedClusterInfoInventorySyncer(mgr ctrl.Manager, inventoryRequester transport.Requester) error {
	clusterInfoPredicate := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return !reflect.DeepEqual(e.ObjectNew.(*clusterinfov1beta1.ManagedClusterInfo).Status,
				e.ObjectOld.(*clusterinfov1beta1.ManagedClusterInfo).Status) ||
				!reflect.DeepEqual(e.ObjectNew.GetLabels(), e.ObjectOld.GetLabels()) ||
				e.ObjectNew.GetGeneration() != e.ObjectOld.GetGeneration()
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// add the annotation to identify the request is creating
			// the annotation won't propagate to the etcd
			annotations := e.Object.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[constants.InventoryResourceCreatingAnnotationlKey] = ""
			e.Object.SetAnnotations(annotations)
			return true
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}

	return ctrl.NewControllerManagedBy(mgr).Named("inventory-managedclusterinfo-controller").
		For(&clusterinfov1beta1.ManagedClusterInfo{}).
		WithEventFilter(clusterInfoPredicate).
		Complete(&ManagedClusterInfoInventorySyncer{
			log:           logger.ZapLogger("managedclusterinfo"),
			runtimeClient: mgr.GetClient(),
			requester:     inventoryRequester,
			clientCN:      requester.GetInventoryClientName(configs.GetLeafHubName()),
		})
}

func GetK8SCluster(clusterInfo *clusterinfov1beta1.ManagedClusterInfo,
	clientCN string,
) *kessel.K8SCluster {
	kesselLabels := []*kessel.ResourceLabel{}
	for key, value := range clusterInfo.Labels {
		kesselLabels = append(kesselLabels, &kessel.ResourceLabel{
			Key:   key,
			Value: value,
		})
	}
	k8sCluster := &kessel.K8SCluster{
		Metadata: &kessel.Metadata{
			ResourceType: "k8s-cluster",
			Labels:       kesselLabels,
		},
		ReporterData: &kessel.ReporterData{
			ReporterType:       kessel.ReporterData_ACM,
			ReporterInstanceId: clientCN,
			ReporterVersion:    configs.GetMCHVersion(),
			LocalResourceId:    clusterInfo.Name,
			ApiHref:            clusterInfo.Spec.MasterEndpoint,
			ConsoleHref:        clusterInfo.Status.ConsoleURL,
		},
		ResourceData: &kessel.K8SClusterDetail{
			ExternalClusterId: clusterInfo.Status.ClusterID,
			KubeVersion:       clusterInfo.Status.Version,
			Nodes:             []*kessel.K8SClusterDetailNodesInner{},
		},
	}

	// platform
	switch clusterInfo.Status.CloudVendor {
	case clusterinfov1beta1.CloudVendorAWS:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	case clusterinfov1beta1.CloudVendorGoogle:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_GCP_UPI
	case clusterinfov1beta1.CloudVendorBareMetal:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_BAREMETAL_UPI
	case clusterinfov1beta1.CloudVendorIBM:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_IBMCLOUD_UPI
	case clusterinfov1beta1.CloudVendorAzure:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	default:
		k8sCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_CLOUD_PLATFORM_OTHER
	}

	// kubevendor, ony have the openshift version
	switch clusterInfo.Status.KubeVendor {
	case clusterinfov1beta1.KubeVendorOpenShift:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_OPENSHIFT
		k8sCluster.ResourceData.VendorVersion = clusterInfo.Status.DistributionInfo.OCP.Version
	case clusterinfov1beta1.KubeVendorEKS:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_EKS
	case clusterinfov1beta1.KubeVendorGKE:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_GKE
	default:
		k8sCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_KUBE_VENDOR_OTHER
	}

	// cluster status
	for _, cond := range clusterInfo.Status.Conditions {
		if cond.Type == clusterv1.ManagedClusterConditionAvailable {
			if cond.Status == metav1.ConditionTrue {
				k8sCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_READY
			} else {
				k8sCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_FAILED
			}
		}
	}

	// nodes
	for _, node := range clusterInfo.Status.NodeList {
		kesselNode := &kessel.K8SClusterDetailNodesInner{
			Name: node.Name,
		}
		cpu, ok := node.Capacity[clusterv1.ResourceCPU]
		if ok {
			kesselNode.Cpu = cpu.String()
		}
		memory, ok := node.Capacity[clusterv1.ResourceMemory]
		if ok {
			kesselNode.Memory = memory.String()
		}

		labels := []*kessel.ResourceLabel{}
		for key, val := range node.Labels {
			if key != "" && val != "" {
				labels = append(labels, &kessel.ResourceLabel{Key: key, Value: val})
			}
		}
		kesselNode.Labels = labels

		k8sCluster.ResourceData.Nodes = append(k8sCluster.ResourceData.Nodes, kesselNode)
	}
	return k8sCluster
}
