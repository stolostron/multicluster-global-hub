package managedclusters

import (
	"testing"

	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestGetK8SCluster(t *testing.T) {
	clusterInfo := createMockClusterInfo("test-cluster", clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
		clusterinfov1beta1.CloudVendorAWS)

	// Call the function
	result := GetK8SCluster(clusterInfo, "guest")

	// Assert the results
	assert.NotNil(t, result)
	assert.Equal(t, "k8s-cluster", result.K8SCluster.Metadata.ResourceType)
	assert.Equal(t, kessel.ReporterData_ACM, result.K8SCluster.ReporterData.ReporterType)
	assert.Equal(t, "https://api.test-cluster.example.com", result.K8SCluster.ReporterData.ApiHref)
	assert.Equal(t, "https://console.test-cluster.example.com", result.K8SCluster.ReporterData.ConsoleHref)
	assert.Equal(t, "test-cluster-id", result.K8SCluster.ResourceData.ExternalClusterId)
	assert.Equal(t, "1.23.0", result.K8SCluster.ResourceData.KubeVersion)
	assert.Equal(t, kessel.K8SClusterDetail_READY, result.K8SCluster.ResourceData.ClusterStatus)
	assert.Equal(t, kessel.K8SClusterDetail_AWS_UPI, result.K8SCluster.ResourceData.CloudPlatform)
	assert.Equal(t, kessel.K8SClusterDetail_OPENSHIFT, result.K8SCluster.ResourceData.KubeVendor)
	assert.Equal(t, "4.10.0", result.K8SCluster.ResourceData.VendorVersion)
}

func TestKubeVendorK8SCluster(t *testing.T) {
	testCases := []struct {
		name            string
		clusterInfo     *clusterinfov1beta1.ManagedClusterInfo
		expectedVendor  kessel.K8SClusterDetail_KubeVendor
		expectedVersion string
	}{
		{
			name: "OpenShift Cluster",
			clusterInfo: createMockClusterInfo("openshift-cluster", clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
				clusterinfov1beta1.CloudVendorAWS),
			expectedVendor:  kessel.K8SClusterDetail_OPENSHIFT,
			expectedVersion: "4.10.0",
		},
		{
			name: "EKS Cluster",
			clusterInfo: createMockClusterInfo("eks-cluster", clusterinfov1beta1.KubeVendorEKS, "",
				clusterinfov1beta1.CloudVendorAzure),
			expectedVendor:  kessel.K8SClusterDetail_EKS,
			expectedVersion: "",
		},
		{
			name: "GKE Cluster",
			clusterInfo: createMockClusterInfo("gke-cluster", clusterinfov1beta1.KubeVendorGKE, "",
				clusterinfov1beta1.CloudVendorGoogle),
			expectedVendor:  kessel.K8SClusterDetail_GKE,
			expectedVersion: "",
		},
		{
			name: "Other Kubernetes Vendor",
			clusterInfo: createMockClusterInfo("other-cluster", "SomeOtherVendor", "",
				clusterinfov1beta1.CloudVendorBareMetal),
			expectedVendor:  kessel.K8SClusterDetail_KUBE_VENDOR_OTHER,
			expectedVersion: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetK8SCluster(tc.clusterInfo, "guest")

			assert.NotNil(t, result)
			assert.Equal(t, tc.expectedVendor, result.K8SCluster.ResourceData.KubeVendor)
			assert.Equal(t, tc.expectedVersion, result.K8SCluster.ResourceData.VendorVersion)
			// Add more assertions for common fields
			assert.Equal(t, "k8s-cluster", result.K8SCluster.Metadata.ResourceType)
			assert.Equal(t, kessel.ReporterData_ACM, result.K8SCluster.ReporterData.ReporterType)
			assert.Equal(t, "https://api.test-cluster.example.com", result.K8SCluster.ReporterData.ApiHref)
			assert.Equal(t, "https://console.test-cluster.example.com", result.K8SCluster.ReporterData.ConsoleHref)
			assert.Equal(t, "test-cluster-id", result.K8SCluster.ResourceData.ExternalClusterId)
			assert.Equal(t, "1.23.0", result.K8SCluster.ResourceData.KubeVersion)
			assert.Equal(t, kessel.K8SClusterDetail_READY, result.K8SCluster.ResourceData.ClusterStatus)
		})
	}
}

func createMockClusterInfo(name string, kubeVendor clusterinfov1beta1.KubeVendorType,
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
