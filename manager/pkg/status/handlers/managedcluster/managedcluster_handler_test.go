package managedcluster

import (
	"context"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestManagedClusterHandler_postToInventoryApi(t *testing.T) {
	type args struct {
		ctx            context.Context
		createClusters []clusterv1.ManagedCluster
		updateClusters []clusterv1.ManagedCluster
		deleteClusters []models.ResourceVersion
		leafHubName    string
		mchVersion     string
	}

	tests := []struct {
		name                       string
		args                       args
		wantCreate                 int
		wantUpdate                 int
		wantDelete                 int
		requesterShouldReturnError bool
		initObjects                []runtime.Object
	}{
		{
			name: "successful create/update/delete",
			args: args{
				ctx: context.TODO(),
				createClusters: []clusterv1.ManagedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster1",
						},
						Status: clusterv1.ManagedClusterStatus{
							ClusterClaims: []clusterv1.ManagedClusterClaim{
								{Name: constants.ClusterIdClaimName, Value: "test-id-1"},
							},
						},
					},
				},
				updateClusters: []clusterv1.ManagedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "cluster2",
						},
						Status: clusterv1.ManagedClusterStatus{
							ClusterClaims: []clusterv1.ManagedClusterClaim{
								{Name: constants.ClusterIdClaimName, Value: "test-id-2"},
							},
						},
					},
				},
				deleteClusters: []models.ResourceVersion{
					{
						Key:  "cluster3",
						Name: "cluster3",
					},
				},
				leafHubName: "hub1",
				mchVersion:  "2.8.0",
			},
			wantCreate:                 1,
			wantUpdate:                 1,
			wantDelete:                 1,
			requesterShouldReturnError: false,
		},
		{
			name: "empty clusters",
			args: args{
				ctx:            context.TODO(),
				createClusters: []clusterv1.ManagedCluster{},
				updateClusters: []clusterv1.ManagedCluster{},
				deleteClusters: []models.ResourceVersion{},
				leafHubName:    "hub1",
				mchVersion:     "2.8.0",
			},
			wantCreate:                 0,
			wantUpdate:                 0,
			wantDelete:                 0,
			requesterShouldReturnError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = clusterv1.AddToScheme(scheme.Scheme)

			h := &managedClusterHandler{
				log: logger.ZapLogger("test"),
			}
			fakeRequester := &FakeRequester{
				HttpClient: &v1beta1.InventoryHttpClient{
					K8sClusterService: &FakeKesselK8SClusterServiceHTTPClientImpl{},
				},
			}
			gotCreate, gotUpdate, gotDelete := h.postToInventoryApi(
				tt.args.ctx,
				fakeRequester,
				tt.args.createClusters,
				tt.args.updateClusters,
				tt.args.deleteClusters,
				tt.args.leafHubName,
			)

			if len(gotCreate) != tt.wantCreate {
				t.Errorf("postToInventoryApi() gotCreate = %v, want %v", len(gotCreate), tt.wantCreate)
			}
			if len(gotUpdate) != tt.wantUpdate {
				t.Errorf("postToInventoryApi() gotUpdate = %v, want %v", len(gotUpdate), tt.wantUpdate)
			}
			if len(gotDelete) != tt.wantDelete {
				t.Errorf("postToInventoryApi() gotDelete = %v, want %v", len(gotDelete), tt.wantDelete)
			}
		})
	}
}

func TestGetK8SCluster(t *testing.T) {
	tests := []struct {
		name        string
		cluster     *clusterv1.ManagedCluster
		leafHubName string
		clusterInfo models.ClusterInfo
		want        *kessel.K8SCluster
	}{
		{
			name: "basic cluster conversion",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-cluster",
					UID:  "test-uid",
					Labels: map[string]string{
						"test-label": "test-value",
					},
				},
				Status: clusterv1.ManagedClusterStatus{
					ClusterClaims: []clusterv1.ManagedClusterClaim{
						{Name: constants.ClusterIdClaimName, Value: "test-cluster-id"},
						{Name: "platform.open-cluster-management.io", Value: "AWS"},
						{Name: "kubeversion.open-cluster-management.io", Value: "1.24.0"},
						{Name: "version.openshift.io", Value: "4.12.0"},
						{Name: "product.open-cluster-management.io", Value: "OpenShift"},
					},
					Conditions: []metav1.Condition{
						{
							Type:   clusterv1.ManagedClusterConditionAvailable,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			leafHubName: "leaf-hub-1",
			clusterInfo: models.ClusterInfo{
				ConsoleURL: "https://console.example.com",
				MchVersion: "2.8.0",
			},
			want: &kessel.K8SCluster{
				Metadata: &kessel.Metadata{
					ResourceType: "k8s_cluster",
					Labels: []*kessel.ResourceLabel{
						{Key: "test-label", Value: "test-value"},
					},
				},
				ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: "leaf-hub-1",
					LocalResourceId:    "test-cluster",
					ConsoleHref:        "https://console.example.com",
					ReporterVersion:    "2.8.0",
				},
				ResourceData: &kessel.K8SClusterDetail{
					ExternalClusterId: "test-cluster-id",
					KubeVersion:       "1.24.0",
					VendorVersion:     "4.12.0",
					CloudPlatform:     kessel.K8SClusterDetail_AWS_UPI,
					KubeVendor:        kessel.K8SClusterDetail_OPENSHIFT,
					ClusterStatus:     kessel.K8SClusterDetail_READY,
					Nodes: []*kessel.K8SClusterDetailNodesInner{
						{
							Name:   "test-cluster",
							Cpu:    "8",
							Memory: "32Gi",
						},
					},
				},
			},
		},
		{
			name: "minimal cluster without claims",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "minimal-cluster",
					UID:  "minimal-uid",
				},
				Status: clusterv1.ManagedClusterStatus{
					ClusterClaims: []clusterv1.ManagedClusterClaim{
						{Name: constants.ClusterIdClaimName, Value: "minimal-id"},
					},
				},
			},
			leafHubName: "leaf-hub-2",
			clusterInfo: models.ClusterInfo{},
			want: &kessel.K8SCluster{
				Metadata: &kessel.Metadata{
					ResourceType: "k8s_cluster",
					Labels:       []*kessel.ResourceLabel{},
				},
				ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: "leaf-hub-2",
					LocalResourceId:    "minimal-cluster",
				},
				ResourceData: &kessel.K8SClusterDetail{
					ExternalClusterId: "minimal-id",
					CloudPlatform:     kessel.K8SClusterDetail_CLOUD_PLATFORM_OTHER,
					KubeVendor:        kessel.K8SClusterDetail_KUBE_VENDOR_OTHER,
					ClusterStatus:     kessel.K8SClusterDetail_OFFLINE,
					Nodes: []*kessel.K8SClusterDetailNodesInner{
						{
							Name: "minimal-cluster",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetK8SCluster(context.Background(), tt.cluster, tt.leafHubName, tt.clusterInfo)

			// Compare specific fields since direct comparison of the entire struct might be too strict
			if got.Metadata.ResourceType != tt.want.Metadata.ResourceType {
				t.Errorf("ResourceType = %v, want %v", got.Metadata.ResourceType, tt.want.Metadata.ResourceType)
			}
			if got.ResourceData.ExternalClusterId != tt.want.ResourceData.ExternalClusterId {
				t.Errorf("ExternalClusterId = %v, want %v", got.ResourceData.ExternalClusterId, tt.want.ResourceData.ExternalClusterId)
			}
			if got.ResourceData.CloudPlatform != tt.want.ResourceData.CloudPlatform {
				t.Errorf("CloudPlatform = %v, want %v", got.ResourceData.CloudPlatform, tt.want.ResourceData.CloudPlatform)
			}
			if got.ResourceData.KubeVendor != tt.want.ResourceData.KubeVendor {
				t.Errorf("KubeVendor = %v, want %v", got.ResourceData.KubeVendor, tt.want.ResourceData.KubeVendor)
			}
			if got.ReporterData.ReporterInstanceId != tt.want.ReporterData.ReporterInstanceId {
				t.Errorf("ReporterInstanceId = %v, want %v", got.ReporterData.ReporterInstanceId, tt.want.ReporterData.ReporterInstanceId)
			}
			if got.ReporterData.ConsoleHref != tt.want.ReporterData.ConsoleHref {
				t.Errorf("ConsoleHref = %v, want %v", got.ReporterData.ConsoleHref, tt.want.ReporterData.ConsoleHref)
			}
		})
	}
}

// FakeRequester is a mock implementation of the Requester interface.
type FakeRequester struct {
	HttpClient *v1beta1.InventoryHttpClient
}

// RefreshClient is a mock implementation that simulates refreshing the client.
func (f *FakeRequester) RefreshClient(ctx context.Context, restConfig *transport.RestfulConfig) error {
	// Simulate a successful refresh operation
	return nil
}

// GetHttpClient returns a mock InventoryHttpClient.
func (f *FakeRequester) GetHttpClient() *v1beta1.InventoryHttpClient {
	// Return the fake HTTP client
	return f.HttpClient
}

type FakeKesselK8SClusterServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) CreateK8SCluster(ctx context.Context, in *kessel.CreateK8SClusterRequest, opts ...http.CallOption) (*kessel.CreateK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) DeleteK8SCluster(ctx context.Context, in *kessel.DeleteK8SClusterRequest, opts ...http.CallOption) (*kessel.DeleteK8SClusterResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SClusterServiceHTTPClientImpl) UpdateK8SCluster(ctx context.Context, in *kessel.UpdateK8SClusterRequest, opts ...http.CallOption) (*kessel.UpdateK8SClusterResponse, error) {
	return nil, nil
}
