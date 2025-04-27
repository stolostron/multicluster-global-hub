package managedcluster

import (
	"context"
	"reflect"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestGetK8SCluster(t *testing.T) {
	type args struct {
		ctx         context.Context
		cluster     *clusterv1.ManagedCluster
		leafHubName string
		initObjects []runtime.Object
	}

	tests := []struct {
		name string
		args args
		want *kessel.K8SCluster
	}{
		{
			name: "Test basic cluster conversion",
			args: args{
				ctx: context.TODO(),
				cluster: &clusterv1.ManagedCluster{
					Status: clusterv1.ManagedClusterStatus{
						ClusterClaims: []clusterv1.ManagedClusterClaim{
							{Name: constants.ClusterIdClaimName, Value: "test-id"},
							{Name: "platform.open-cluster-management.io", Value: "AWS"},
							{Name: "kubeversion.open-cluster-management.io", Value: "1.24"},
							{Name: "version.openshift.io", Value: "4.12"},
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
				leafHubName: "hub1",
			},
			want: &kessel.K8SCluster{
				Metadata: &kessel.Metadata{
					ResourceType: "k8s_cluster",
					Labels:       []*kessel.ResourceLabel{},
				},
				ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: "hub1",
				},
				ResourceData: &kessel.K8SClusterDetail{
					ExternalClusterId: "test-id",
					CloudPlatform:     kessel.K8SClusterDetail_AWS_UPI,
					KubeVersion:       "1.24",
					VendorVersion:     "4.12",
					KubeVendor:        kessel.K8SClusterDetail_OPENSHIFT,
					ClusterStatus:     kessel.K8SClusterDetail_READY,
					Nodes: []*kessel.K8SClusterDetailNodesInner{
						{
							Name: "",
						},
					},
				},
			},
		},
		{
			name: "Test minimal cluster with defaults",
			args: args{
				ctx: context.TODO(),
				cluster: &clusterv1.ManagedCluster{
					Status: clusterv1.ManagedClusterStatus{
						ClusterClaims: []clusterv1.ManagedClusterClaim{
							{Name: constants.ClusterIdClaimName, Value: "test-id-2"},
							{Name: "kubeversion.open-cluster-management.io", Value: "1.25"},
							{Name: "version.open-cluster-management.io", Value: "2.8.0"},
						},
						Conditions: []metav1.Condition{
							{
								Type:   clusterv1.ManagedClusterConditionAvailable,
								Status: metav1.ConditionTrue,
							},
						},
					},
				},
				leafHubName: "hub2",
				initObjects: []runtime.Object{
					&clusterv1.ManagedCluster{
						ObjectMeta: metav1.ObjectMeta{
							Name: "hub2",
						},
						Status: clusterv1.ManagedClusterStatus{
							ClusterClaims: []clusterv1.ManagedClusterClaim{
								{Name: "consoleurl.cluster.open-cluster-management.io", Value: "https://consoleurl"},
								{Name: "apiserverurl.openshift.io", Value: "http://apiserverurl"},
								{Name: "version.open-cluster-management.io", Value: "2.8.0"},
							},
						},
					},
				},
			},
			want: &kessel.K8SCluster{
				Metadata: &kessel.Metadata{
					ResourceType: "k8s_cluster",
					Labels:       []*kessel.ResourceLabel{},
				},
				ReporterData: &kessel.ReporterData{
					ReporterType:       kessel.ReporterData_ACM,
					ReporterInstanceId: "hub2",
					ReporterVersion:    "2.8.0",
					ConsoleHref:        "https://consoleurl",
					ApiHref:            "http://apiserverurl",
				},
				ResourceData: &kessel.K8SClusterDetail{
					ExternalClusterId: "test-id-2",
					CloudPlatform:     kessel.K8SClusterDetail_CLOUD_PLATFORM_OTHER,
					KubeVersion:       "1.25",
					VendorVersion:     "1.25",
					KubeVendor:        kessel.K8SClusterDetail_KUBE_VENDOR_OTHER,
					ClusterStatus:     kessel.K8SClusterDetail_READY,
					Nodes: []*kessel.K8SClusterDetailNodesInner{
						{
							Name: "",
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clusterv1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.args.initObjects...).Build()

			got := GetK8SCluster(tt.args.ctx, tt.args.cluster, tt.args.leafHubName, fakeClient)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetK8SCluster() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
			clusterv1.AddToScheme(scheme.Scheme)
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithRuntimeObjects(tt.initObjects...).Build()

			h := &managedClusterHandler{
				c:   fakeClient,
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
