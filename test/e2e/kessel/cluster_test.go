package kessel

import (
	"fmt"
	"math/rand"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kesselresources "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/controllers/inventory/managedclusterinfo"
)

// https://github.com/project-kessel/docs/blob/main/src/content/docs/inventory/kafka-event.md
var _ = Describe("kafka-event: cluster API", Ordered, func() {
	var localResourceId string
	var k8sCluster *kesselresources.K8SCluster
	BeforeAll(func() {
		// also is the name of managedclusterinfo
		localResourceId = fmt.Sprintf("test-cluster-%d", rand.Intn(100000))
		clusterInfo := mockManagedClusterInfo(localResourceId, clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
			clusterinfov1beta1.CloudVendorAWS)
		cluster := createMockCluster(localResourceId, "OpenShift", "4.10.0", "AWS", "1.23.0")
		k8sCluster = managedclusterinfo.GetK8SCluster(clusterInfo, cluster, "guest")
	})

	It("Create", func() {
		_, err := inventoryClient.GetHttpClient().K8sClusterService.CreateK8SCluster(ctx,
			&kesselresources.CreateK8SClusterRequest{K8SCluster: k8sCluster})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s-cluster.created"
			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localResourceId,
					data.ReporterData.LocalResourceId)
			}
			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Update", func() {
		k8sCluster.ResourceData.ClusterStatus = kesselresources.K8SClusterDetail_FAILED
		_, err := inventoryClient.GetHttpClient().K8sClusterService.UpdateK8SCluster(ctx,
			&kesselresources.UpdateK8SClusterRequest{K8SCluster: k8sCluster})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s-cluster.updated"

			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localResourceId,
					data.ReporterData.LocalResourceId)
			}

			if data.ResourceData["cluster_status"] != "FAILED" {
				return fmt.Errorf("K8SClusterStatus(%s), want %s, but got %s", clusterType, "FAILED",
					data.ResourceData["cluster_status"])
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})

	It("Delete", func() {
		_, err := inventoryClient.GetHttpClient().K8sClusterService.DeleteK8SCluster(ctx,
			&kesselresources.DeleteK8SClusterRequest{ReporterData: k8sCluster.ReporterData})
		Expect(err).To(Succeed())

		Eventually(func() error {
			clusterType := "redhat.inventory.resources.k8s-cluster.deleted"

			event, ok := receivedEvents[clusterType]
			if !ok {
				return fmt.Errorf("not recieve event %s:%s", clusterType, localResourceId)
			}
			data := &ResourceData{}
			err := event.DataAs(data)
			if err != nil {
				return fmt.Errorf("failed to decode the event: %v", event)
			}

			if data.ReporterData.LocalResourceId != localResourceId {
				return fmt.Errorf("LocalResourceId(%s), want %s, but got %s", clusterType, localResourceId,
					data.ReporterData.LocalResourceId)
			}

			return nil
		}, TIMEOUT, INTERVAL).Should(Succeed())
	})
})

func mockManagedClusterInfo(name string, kubeVendor clusterinfov1beta1.KubeVendorType,
	vendorVersion string, platform clusterinfov1beta1.CloudVendorType,
) *clusterinfov1beta1.ManagedClusterInfo {
	clusterInfo := &clusterinfov1beta1.ManagedClusterInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterinfov1beta1.ClusterInfoSpec{
			MasterEndpoint: "https://api.test-cluster.example.com",
		},
		Status: clusterinfov1beta1.ClusterInfoStatus{
			ClusterID:   "test-cluster-id",
			Version:     "1.23.0",
			ConsoleURL:  "https://console.test-cluster.example.com",
			CloudVendor: platform,
			KubeVendor:  kubeVendor,
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1.ManagedClusterConditionAvailable,
					Status: metav1.ConditionTrue,
				},
			},
			NodeList: []clusterinfov1beta1.NodeStatus{
				{
					Name: "ip-10-0-14-217.ec2.internal",
					Capacity: clusterinfov1beta1.ResourceList{
						clusterv1.ResourceCPU:    resource.MustParse("16"),
						clusterv1.ResourceMemory: resource.MustParse("64453796Ki"),
					},
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "m6a.4xlarge",
					},
				},
			},
		},
	}

	if kubeVendor == clusterinfov1beta1.KubeVendorOpenShift {
		clusterInfo.Status.DistributionInfo = clusterinfov1beta1.DistributionInfo{
			OCP: clusterinfov1beta1.OCPDistributionInfo{
				Version: vendorVersion,
			},
		}
	}

	return clusterInfo
}

func createMockCluster(name, kubeVendor, vendorVersion, platform, kubeVersion string,
) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: clusterv1.ManagedClusterSpec{
			HubAcceptsClient: true,
		},
		Status: clusterv1.ManagedClusterStatus{
			ClusterClaims: []clusterv1.ManagedClusterClaim{
				{
					Name:  "id.k8s.io",
					Value: uuid.New().String(),
				},
				{
					Name:  "platform.open-cluster-management.io",
					Value: platform,
				},
				{
					Name:  "kubeversion.open-cluster-management.io",
					Value: kubeVersion,
				},
				{
					Name:  "version.openshift.io",
					Value: vendorVersion,
				},
				{
					Name:  "product.open-cluster-management.io",
					Value: kubeVendor,
				},
			},
		},
	}
}
