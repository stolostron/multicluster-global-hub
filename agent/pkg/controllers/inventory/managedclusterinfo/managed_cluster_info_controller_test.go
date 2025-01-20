package managedclusterinfo

import (
	"context"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/google/uuid"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/time"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestManagedClusterInfoCtrlReconcile(t *testing.T) {
	// Setup scheme and mock requester
	scheme := runtime.NewScheme()
	_ = clusterinfov1beta1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	mockRequester := &MockRequest{}

	// Define test cases
	creatingTime := metav1.Now()
	deletintTime := metav1.NewTime(time.Now().Time)
	tests := []struct {
		name           string
		clusterInfo    *clusterinfov1beta1.ManagedClusterInfo
		cluster        *clusterv1.ManagedCluster
		expectedResult reconcile.Result
		expectedError  bool
	}{
		{
			name: "Creating new cluster",
			clusterInfo: &clusterinfov1beta1.ManagedClusterInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-cluster",
					Annotations: map[string]string{
						constants.InventoryResourceCreatingAnnotationlKey: "",
					},
				},
			},
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
					UID:  types.UID(uuid.New().String()),
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "Updating existing cluster",
			clusterInfo: &clusterinfov1beta1.ManagedClusterInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					Namespace:         "test-cluster",
					CreationTimestamp: creatingTime,
				},
			},
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					CreationTimestamp: creatingTime,
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "Deleting cluster",
			clusterInfo: &clusterinfov1beta1.ManagedClusterInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					Namespace:         "test-cluster",
					CreationTimestamp: creatingTime,
					DeletionTimestamp: &deletintTime,
					Finalizers:        []string{constants.InventoryResourceFinalizer},
				},
			},
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster",
					CreationTimestamp: creatingTime,
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the fake Kubernetes client with test objects
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.clusterInfo, tt.cluster).Build()

			// Create the controller with the mock requester and fake client
			r := &ManagedClusterInfoInventorySyncer{
				runtimeClient: fakeClient,
				requester:     mockRequester,
				clientCN:      "test-clientCN",
			}

			// Call the Reconcile method
			result, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-cluster", Namespace: "test-cluster"},
			})

			// Assert results
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

type MockRequest struct{}

func (c *MockRequest) RefreshClient(ctx context.Context, restfulConn *transport.RestfulConfig) error {
	return nil
}

func (c *MockRequest) GetHttpClient() *v1beta1.InventoryHttpClient {
	return &v1beta1.InventoryHttpClient{
		K8sClusterService: &ClusterServiceClient{},
	}
}

type ClusterServiceClient struct{}

func (c *ClusterServiceClient) CreateK8SCluster(ctx context.Context, in *kessel.CreateK8SClusterRequest,
	opts ...http.CallOption,
) (*kessel.CreateK8SClusterResponse, error) {
	return nil, nil
}

func (c *ClusterServiceClient) UpdateK8SCluster(ctx context.Context, in *kessel.UpdateK8SClusterRequest,
	opts ...http.CallOption,
) (*kessel.UpdateK8SClusterResponse, error) {
	return nil, nil
}

func (c *ClusterServiceClient) DeleteK8SCluster(ctx context.Context, in *kessel.DeleteK8SClusterRequest,
	opts ...http.CallOption,
) (*kessel.DeleteK8SClusterResponse, error) {
	return nil, nil
}

func TestGetK8SClusterInfo(t *testing.T) {
	clusterInfo := createMockClusterInfo("test-cluster", clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
		clusterinfov1beta1.CloudVendorAWS)

	cluster := createMockCluster("test-cluster", "OpenShift", "4.10.0", "AWS", "1.23.0")
	// Call the function
	k8sCluster := GetK8SCluster(context.Background(), clusterInfo, cluster, "guest", nil)

	// Assert the results
	assert.NotNil(t, k8sCluster)
	assert.Equal(t, "k8s_cluster", k8sCluster.Metadata.ResourceType)
	assert.Equal(t, kessel.ReporterData_ACM, k8sCluster.ReporterData.ReporterType)
	assert.Equal(t, "23e5ae9e-c6b2-4793-be6b-2e52f870df10", k8sCluster.ResourceData.ExternalClusterId)
	assert.Equal(t, "1.23.0", k8sCluster.ResourceData.KubeVersion)
	assert.Equal(t, kessel.K8SClusterDetail_READY, k8sCluster.ResourceData.ClusterStatus)
	assert.Equal(t, kessel.K8SClusterDetail_AWS_UPI, k8sCluster.ResourceData.CloudPlatform)
	assert.Equal(t, kessel.K8SClusterDetail_OPENSHIFT, k8sCluster.ResourceData.KubeVendor)
	assert.Equal(t, "4.10.0", k8sCluster.ResourceData.VendorVersion)
}

func TestKubeVendorK8SCluster(t *testing.T) {
	testCases := []struct {
		name            string
		clusterInfo     *clusterinfov1beta1.ManagedClusterInfo
		cluster         *clusterv1.ManagedCluster
		expectedVendor  kessel.K8SClusterDetail_KubeVendor
		expectedVersion string
	}{
		{
			name: "OpenShift Cluster",
			clusterInfo: createMockClusterInfo("openshift-cluster", clusterinfov1beta1.KubeVendorOpenShift, "4.10.0",
				clusterinfov1beta1.CloudVendorAWS),
			cluster:         createMockCluster("openshift-cluster", "OpenShift", "4.10.0", "Amazon", "1.23.0"),
			expectedVendor:  kessel.K8SClusterDetail_OPENSHIFT,
			expectedVersion: "4.10.0",
		},
		{
			name: "EKS Cluster",
			clusterInfo: createMockClusterInfo("eks-cluster", clusterinfov1beta1.KubeVendorEKS, "",
				clusterinfov1beta1.CloudVendorAzure),
			cluster:         createMockCluster("eks-cluster", "EKS", "", "Azure", "1.23.0"),
			expectedVendor:  kessel.K8SClusterDetail_EKS,
			expectedVersion: "1.23.0",
		},
		{
			name: "GKE Cluster",
			clusterInfo: createMockClusterInfo("gke-cluster", clusterinfov1beta1.KubeVendorGKE, "",
				clusterinfov1beta1.CloudVendorGoogle),
			cluster:         createMockCluster("gke-cluster", "GKE", "", "Google", "1.23.0"),
			expectedVendor:  kessel.K8SClusterDetail_GKE,
			expectedVersion: "1.23.0",
		},
		{
			name: "Other Kubernetes Vendor",
			clusterInfo: createMockClusterInfo("other-cluster", "SomeOtherVendor", "",
				clusterinfov1beta1.CloudVendorBareMetal),
			cluster:         createMockCluster("other-cluster", "SomeOtherVendor", "", "BareMetal", "1.23.0"),
			expectedVendor:  kessel.K8SClusterDetail_KUBE_VENDOR_OTHER,
			expectedVersion: "1.23.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			k8sCluster := GetK8SCluster(context.Background(), tc.clusterInfo, tc.cluster, "guest", nil)

			assert.NotNil(t, k8sCluster)
			assert.Equal(t, tc.expectedVendor, k8sCluster.ResourceData.KubeVendor)
			assert.Equal(t, tc.expectedVersion, k8sCluster.ResourceData.VendorVersion)
			// Add more assertions for common fields
			assert.Equal(t, "k8s_cluster", k8sCluster.Metadata.ResourceType)
			assert.Equal(t, kessel.ReporterData_ACM, k8sCluster.ReporterData.ReporterType)
			assert.Equal(t, "23e5ae9e-c6b2-4793-be6b-2e52f870df10", k8sCluster.ResourceData.ExternalClusterId)
			assert.Equal(t, "1.23.0", k8sCluster.ResourceData.KubeVersion)
			assert.Equal(t, kessel.K8SClusterDetail_READY, k8sCluster.ResourceData.ClusterStatus)
		})
	}
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
					Value: "23e5ae9e-c6b2-4793-be6b-2e52f870df10",
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
			ClusterID:   "23e5ae9e-c6b2-4793-be6b-2e52f870df10",
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
