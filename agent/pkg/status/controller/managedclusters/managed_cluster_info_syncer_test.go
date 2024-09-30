package managedclusters

import (
	"context"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/time"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestManagedClusterInfoCtrlReconcile(t *testing.T) {
	// Setup scheme and mock requester
	scheme := runtime.NewScheme()
	_ = clusterinfov1beta1.AddToScheme(scheme)
	mockRequester := &MockRequest{}

	// Define test cases
	creatingTime := metav1.Now()
	deletintTime := metav1.NewTime(time.Now().Time)
	tests := []struct {
		name           string
		clusterInfo    *clusterinfov1beta1.ManagedClusterInfo
		expectedResult reconcile.Result
		expectedError  bool
	}{
		{
			name: "Creating new cluster",
			clusterInfo: &clusterinfov1beta1.ManagedClusterInfo{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
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
					Namespace:         "default",
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
					Namespace:         "default",
					CreationTimestamp: creatingTime,
					DeletionTimestamp: &deletintTime,
					Finalizers:        []string{"test"},
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the fake Kubernetes client with test objects
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.clusterInfo).Build()

			// Create the controller with the mock requester and fake client
			r := &ManagedClusterInfoCtrl{
				runtimeClient: fakeClient,
				requester:     mockRequester,
				clientCN:      "test-clientCN",
			}

			// Call the Reconcile method
			result, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-cluster", Namespace: "default"},
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
