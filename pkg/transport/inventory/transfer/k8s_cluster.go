package transfer

import (
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func GetK8SCluster(clusterInfo *clusterinfov1beta1.ManagedClusterInfo) *kessel.CreateK8SClusterRequest {
	clusterRequest := &kessel.CreateK8SClusterRequest{
		K8SCluster: &kessel.K8SCluster{
			Metadata: &kessel.Metadata{
				ResourceType: "k8s-cluster",
			},
			ReporterData: &kessel.ReporterData{
				ReporterType:       kessel.ReporterData_ACM,
				ReporterInstanceId: "guest",
				ReporterVersion:    "0.1",
				LocalResourceId:    "1",
				ApiHref:            clusterInfo.Spec.MasterEndpoint,
				ConsoleHref:        clusterInfo.Status.ConsoleURL,
			},
			ResourceData: &kessel.K8SClusterDetail{
				ExternalClusterId: clusterInfo.Status.ClusterID,
				KubeVersion:       clusterInfo.Status.Version,
				Nodes:             []*kessel.K8SClusterDetailNodesInner{},
			},
		},
	}

	// platform
	switch clusterInfo.Status.CloudVendor {
	case clusterinfov1beta1.CloudVendorAWS:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	case clusterinfov1beta1.CloudVendorGoogle:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_GCP_UPI
	case clusterinfov1beta1.CloudVendorBareMetal:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_BAREMETAL_UPI
	case clusterinfov1beta1.CloudVendorIBM:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_IBMCLOUD_UPI
	case clusterinfov1beta1.CloudVendorAzure:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_AWS_UPI
	default:
		clusterRequest.K8SCluster.ResourceData.CloudPlatform = kessel.K8SClusterDetail_CLOUD_PLATFORM_OTHER
	}

	// kubevendor, ony have the openshift version
	switch clusterInfo.Status.KubeVendor {
	case clusterinfov1beta1.KubeVendorOpenShift:
		clusterRequest.K8SCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_OPENSHIFT
		clusterRequest.K8SCluster.ResourceData.VendorVersion = clusterInfo.Status.DistributionInfo.OCP.Version
	case clusterinfov1beta1.KubeVendorEKS:
		clusterRequest.K8SCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_EKS
	case clusterinfov1beta1.KubeVendorGKE:
		clusterRequest.K8SCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_GKE
	default:
		clusterRequest.K8SCluster.ResourceData.KubeVendor = kessel.K8SClusterDetail_KUBE_VENDOR_OTHER
	}

	// cluster status
	for _, cond := range clusterInfo.Status.Conditions {
		if cond.Type == clusterv1.ManagedClusterConditionAvailable {
			if cond.Status == metav1.ConditionTrue {
				clusterRequest.K8SCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_READY
			} else {
				clusterRequest.K8SCluster.ResourceData.ClusterStatus = kessel.K8SClusterDetail_FAILED
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

		clusterRequest.K8SCluster.ResourceData.Nodes = append(clusterRequest.K8SCluster.ResourceData.Nodes, kesselNode)
	}
	return clusterRequest
}
