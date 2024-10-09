package policies

import (
	"context"
	"testing"

	http "github.com/go-kratos/kratos/v2/transport/http"
	kessel "github.com/project-kessel/inventory-api/api/kessel/inventory/v1beta1/resources"
	"github.com/project-kessel/inventory-client-go/v1beta1"
	"github.com/stretchr/testify/assert"
	"helm.sh/helm/v3/pkg/time"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func TestPolicyControllerReconcile(t *testing.T) {
	// Setup scheme and mock requester
	scheme := runtime.NewScheme()
	_ = policiesv1.AddToScheme(scheme)
	mockRequester := &MockRequest{}

	// Define test cases
	deletintTime := metav1.NewTime(time.Now().Time)
	tests := []struct {
		name           string
		policy         *policiesv1.Policy
		expectedResult reconcile.Result
		expectedError  bool
	}{
		{
			name: "Creating new policy",
			policy: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "Updating existing policy",
			policy: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-policy",
					Namespace: "default",
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
		{
			name: "Deleting cluster",
			policy: &policiesv1.Policy{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-policy",
					Namespace:         "default",
					DeletionTimestamp: &deletintTime,
					Finalizers:        []string{"test"},
				},
			},
			expectedResult: reconcile.Result{},
			expectedError:  false,
		},
	}

	config := &rest.Config{
		Host: "localhost",
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}

	mgr, err := manager.New(config, manager.Options{
		NewClient: func(config *rest.Config, options client.Options) (client.Client, error) {
			return fake.NewClientBuilder().WithScheme(scheme).WithObjects().Build(), nil
		},
		Scheme: scheme,
	})
	assert.NoError(t, err)
	assert.NoError(t, AddPolicyController(mgr, nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the fake Kubernetes client with test objects
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.policy).Build()

			// Create the controller with the mock requester and fake client
			r := &PolicyController{
				runtimeClient:      fakeClient,
				requester:          mockRequester,
				reporterInstanceId: "test-clientCN",
			}

			// Call the Reconcile method
			result, err := r.Reconcile(context.TODO(), reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-policy", Namespace: "default"},
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
		PolicyServiceClient: &PolicyServiceClient{},
	}
}

type PolicyServiceClient struct{}

func (p *PolicyServiceClient) CreateK8SPolicy(ctx context.Context, in *kessel.CreateK8SPolicyRequest,
	opts ...http.CallOption,
) (*kessel.CreateK8SPolicyResponse, error) {
	return nil, nil
}

func (p *PolicyServiceClient) UpdateK8SPolicy(ctx context.Context, in *kessel.UpdateK8SPolicyRequest,
	opts ...http.CallOption,
) (*kessel.UpdateK8SPolicyResponse, error) {
	return nil, nil
}

func (p *PolicyServiceClient) DeleteK8SPolicy(ctx context.Context, in *kessel.DeleteK8SPolicyRequest,
	opts ...http.CallOption,
) (*kessel.DeleteK8SPolicyResponse, error) {
	return nil, nil
}
