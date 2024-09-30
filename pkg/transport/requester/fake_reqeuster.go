package requester

import (
	"context"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

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
