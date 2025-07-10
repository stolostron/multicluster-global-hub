package policy

import (
	"context"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

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

type FakeKesselK8SPolicyServiceHTTPClientImpl struct{}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) CreateK8SPolicy(ctx context.Context, in *kessel.CreateK8SPolicyRequest, opts ...http.CallOption) (*kessel.CreateK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) DeleteK8SPolicy(ctx context.Context, in *kessel.DeleteK8SPolicyRequest, opts ...http.CallOption) (*kessel.DeleteK8SPolicyResponse, error) {
	return nil, nil
}

func (c *FakeKesselK8SPolicyServiceHTTPClientImpl) UpdateK8SPolicy(ctx context.Context, in *kessel.UpdateK8SPolicyRequest, opts ...http.CallOption) (*kessel.UpdateK8SPolicyResponse, error) {
	return nil, nil
}
