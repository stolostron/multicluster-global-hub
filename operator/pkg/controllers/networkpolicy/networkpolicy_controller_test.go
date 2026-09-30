package networkpolicy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	fakerest "k8s.io/client-go/rest/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// assertPredicateAllEvents asserts Create, Update, and Delete predicate results for the same expected value.
func assertPredicateAllEvents(t *testing.T, pred predicate.Funcs, obj client.Object, want bool) {
	t.Helper()
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}), "CreateFunc")
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}), "UpdateFunc")
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}), "DeleteFunc")
}

func TestNetworkPolicyPredicate(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		wantBool bool
	}{
		{
			name: "default-deny-all network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyDefaultDenyAll,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "allow-dns-and-api network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyAllowDNSAndAPI,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "multicluster-global-hub-operator network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyOperator,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "volsync-mover network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyVolsyncMover,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "other network policy should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-policy",
					Namespace: namespace,
				},
			},
			wantBool: false,
		},
		{
			name: "wrong namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyDefaultDenyAll,
					Namespace: "wrong-namespace",
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPredicateAllEvents(t, networkPolicyPred, tt.obj, tt.wantBool)
		})
	}
}

func TestByoSecretPredicate(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	tests := []struct {
		name     string
		obj      *corev1.Secret
		wantBool bool
	}{
		{
			name: "BYO storage secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHStorageSecretName,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "BYO transport secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHTransportSecretName,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "unrelated secret should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-secret",
					Namespace: namespace,
				},
			},
			wantBool: false,
		},
		{
			name: "BYO storage secret in wrong namespace should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHStorageSecretName,
					Namespace: "wrong-namespace",
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPredicateAllEvents(t, byoSecretPred, tt.obj, tt.wantBool)
		})
	}
}

func TestIsWatchedByoSecretCustomNamespace(t *testing.T) {
	originalNS := networkPolicyWatchNamespace
	t.Cleanup(func() { networkPolicyWatchNamespace = originalNS })

	customNamespace := "custom-namespace"
	networkPolicyWatchNamespace = customNamespace

	tests := []struct {
		name     string
		obj      *corev1.Secret
		wantBool bool
	}{
		{
			name: "BYO secret in custom namespace should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHStorageSecretName,
					Namespace: customNamespace,
				},
			},
			wantBool: true,
		},
		{
			name: "BYO secret in default namespace should NOT match when custom namespace is set",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHStorageSecretName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantBool, isWatchedByoSecret(tt.obj))
		})
	}
}

func TestIsResourceRemoved(t *testing.T) {
	r := &NetworkPolicyReconciler{}
	assert.True(t, r.IsResourceRemoved())
}

func TestStartControllerSingleton(t *testing.T) {
	// Save and restore the singleton so we don't affect other tests.
	original := networkPolicyController
	t.Cleanup(func() { networkPolicyController = original })

	existing := &NetworkPolicyReconciler{}
	networkPolicyController = existing

	// Second call must return the pre-existing instance without error.
	controller, err := StartController(config.ControllerOption{})
	assert.NoError(t, err)
	assert.Equal(t, existing, controller)
}

func TestIsWatchedNetworkPolicyCustomNamespace(t *testing.T) {
	// Set a custom watch namespace and verify predicate uses it instead of the default.
	originalNS := networkPolicyWatchNamespace
	t.Cleanup(func() { networkPolicyWatchNamespace = originalNS })

	customNamespace := "custom-namespace"
	networkPolicyWatchNamespace = customNamespace

	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		wantBool bool
	}{
		{
			name: "watched NP in custom namespace should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyDefaultDenyAll,
					Namespace: customNamespace,
				},
			},
			wantBool: true,
		},
		{
			name: "watched NP in default namespace should NOT match when custom namespace is set",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      NetworkPolicyDefaultDenyAll,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: false,
		},
		{
			name: "unknown NP in custom namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "unrelated-policy",
					Namespace: customNamespace,
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantBool, isWatchedNetworkPolicy(tt.obj))
		})
	}
}

func TestVolsyncCRDPredicate(t *testing.T) {
	volsyncCRD := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: volsyncReplicationSourceCRD}}
	other := &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "other"}}
	assertPredicateAllEvents(t, volsyncCRDPred, volsyncCRD, true)
	assertPredicateAllEvents(t, volsyncCRDPred, other, false)
}

func TestWithoutVolsyncMover(t *testing.T) {
	mover := &unstructured.Unstructured{}
	mover.SetKind("NetworkPolicy")
	mover.SetName(NetworkPolicyVolsyncMover)
	other := &unstructured.Unstructured{}
	other.SetKind("NetworkPolicy")
	other.SetName(NetworkPolicyDefaultDenyAll)

	filtered := withoutVolsyncMover([]*unstructured.Unstructured{mover, other})
	require.Len(t, filtered, 1)
	assert.Equal(t, NetworkPolicyDefaultDenyAll, filtered[0].GetName())
}

type fakeGroupLister struct {
	client rest.Interface
}

func (f fakeGroupLister) RESTClient() rest.Interface {
	return f.client
}

func newVolsyncRESTClient(statusCode int, err error) rest.Interface {
	body := `{"kind":"APIResourceList","apiVersion":"v1",` +
		`"groupVersion":"volsync.backube/v1alpha1","resources":[]}`
	if statusCode == http.StatusNotFound {
		body = `{"kind":"Status","apiVersion":"v1","metadata":{},` +
			`"status":"Failure","message":"not found","reason":"NotFound","code":404}`
	}
	s := runtime.NewScheme()
	metav1.AddToGroupVersion(s, schema.GroupVersion{Version: "v1"})
	return &fakerest.RESTClient{
		NegotiatedSerializer: serializer.NewCodecFactory(s),
		GroupVersion:         schema.GroupVersion{Version: "v1"},
		Err:                  err,
		Resp: &http.Response{
			StatusCode: statusCode,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}
}

func TestVolsyncAPIInstalled(t *testing.T) {
	ctx := context.Background()

	installed, err := volsyncAPIInstalled(ctx, fakeGroupLister{client: newVolsyncRESTClient(http.StatusOK, nil)})
	require.NoError(t, err)
	assert.True(t, installed, "a successful discovery response means VolSync is installed")

	installed, err = volsyncAPIInstalled(ctx, fakeGroupLister{client: newVolsyncRESTClient(http.StatusNotFound, nil)})
	require.NoError(t, err)
	assert.False(t, installed, "a missing VolSync API means the mover policy must not be created")

	installed, err = volsyncAPIInstalled(ctx, fakeGroupLister{
		client: newVolsyncRESTClient(http.StatusOK, errors.New("discovery unavailable")),
	})
	require.ErrorContains(t, err, "discovery unavailable")
	assert.False(t, installed)

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	installed, err = volsyncAPIInstalled(cancelled, fakeGroupLister{client: newVolsyncRESTClient(http.StatusOK, nil)})
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, installed, "a cancelled reconcile must not treat discovery as successful")
}

func TestClassifyVolsyncDiscovery(t *testing.T) {
	installed, err := classifyVolsyncDiscovery(nil)
	require.NoError(t, err)
	assert.True(t, installed)

	installed, err = classifyVolsyncDiscovery(apierrors.NewNotFound(schema.GroupResource{}, volsyncGroupVersion))
	require.NoError(t, err)
	assert.False(t, installed)

	installed, err = classifyVolsyncDiscovery(&discovery.ErrGroupDiscoveryFailed{
		Groups: map[schema.GroupVersion]error{
			{Group: "volsync.backube", Version: "v1alpha1"}: apierrors.NewNotFound(
				schema.GroupResource{}, volsyncGroupVersion,
			),
		},
	})
	require.NoError(t, err)
	assert.False(t, installed, "a failed lookup of the VolSync group means it is not installed")

	installed, err = classifyVolsyncDiscovery(errors.New("discovery unavailable"))
	require.EqualError(t, err, "discovery unavailable")
	assert.False(t, installed)

	installed, err = classifyVolsyncDiscovery(&discovery.ErrGroupDiscoveryFailed{
		Groups: map[schema.GroupVersion]error{
			{Group: "other.example.com", Version: "v1"}: errors.New("failed"),
		},
	})
	require.Error(t, err)
	assert.False(t, installed, "discovery failures for other APIs must be retried")
}

// clientManager stubs the manager methods the VolSync helpers call.
type clientManager struct {
	ctrl.Manager
	c      client.Client
	config *rest.Config
	scheme *runtime.Scheme
}

func (m *clientManager) GetClient() client.Client {
	return m.c
}

func (m *clientManager) GetConfig() *rest.Config {
	return m.config
}

func (m *clientManager) GetScheme() *runtime.Scheme {
	return m.scheme
}

// stubKubeClient serves a fixed discovery response to the VolSync install check.
type stubKubeClient struct {
	kubernetes.Interface
	discovery discovery.DiscoveryInterface
}

func (c *stubKubeClient) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

type stubDiscovery struct {
	discovery.DiscoveryInterface
	restClient rest.Interface
}

func (s *stubDiscovery) RESTClient() rest.Interface {
	return s.restClient
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func volsyncTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, networkingv1.AddToScheme(s))
	require.NoError(t, v1alpha4.AddToScheme(s))
	return s
}

func reconcilerWithClient(c client.Client) *NetworkPolicyReconciler {
	return &NetworkPolicyReconciler{Manager: &clientManager{c: c}}
}

func ownedVolsyncMoverPolicy(namespace string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:            NetworkPolicyVolsyncMover,
			Namespace:       namespace,
			UID:             "volsync-mover-uid",
			ResourceVersion: "1",
			Labels: map[string]string{
				networkPolicyManagedByLabel: networkPolicyManagedByValue,
			},
		},
	}
}

func TestEnqueueForVolsyncCRD(t *testing.T) {
	ctx := context.Background()
	original := config.GetMGHNamespacedName()
	t.Cleanup(func() { config.SetMGHNamespacedName(original) })

	volsyncCRD := &unstructured.Unstructured{}
	volsyncCRD.SetName(volsyncReplicationSourceCRD)
	other := &unstructured.Unstructured{}
	other.SetName("other")

	config.SetMGHNamespacedName(types.NamespacedName{Name: "hub", Namespace: "gh"})
	r := &NetworkPolicyReconciler{}
	assert.Nil(t, r.enqueueForVolsyncCRD(ctx, other), "unrelated CRDs must not enqueue reconciliation")
	reqs := r.enqueueForVolsyncCRD(ctx, volsyncCRD)
	require.Len(t, reqs, 1)
	assert.Equal(t, "hub", reqs[0].Name)
	assert.Equal(t, "gh", reqs[0].Namespace)

	config.SetMGHNamespacedName(types.NamespacedName{})
	failing := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		List: func(
			context.Context, client.WithWatch, client.ObjectList, ...client.ListOption,
		) error {
			return errors.New("list failed")
		},
	}).Build()
	reqs = reconcilerWithClient(failing).enqueueForVolsyncCRD(ctx, volsyncCRD)
	require.Len(t, reqs, 1, "a hub lookup failure must still enqueue so Reconcile can retry it")
	assert.NotEmpty(t, reqs[0].Name)
	assert.NotEmpty(t, reqs[0].Namespace)
}

func TestDeleteVolsyncMoverPolicy(t *testing.T) {
	ctx := context.Background()
	scheme := volsyncTestScheme(t)
	namespace := "gh"

	t.Run("missing policy", func(t *testing.T) {
		r := reconcilerWithClient(fake.NewClientBuilder().WithScheme(scheme).Build())
		require.NoError(t, r.deleteVolsyncMoverPolicy(ctx, namespace),
			"a missing mover policy is already the desired state")
	})

	t.Run("policy owned by another controller is kept", func(t *testing.T) {
		foreign := ownedVolsyncMoverPolicy(namespace)
		foreign.Labels = nil
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreign).Build()
		require.NoError(t, reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace))

		got := &networkingv1.NetworkPolicy{}
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(foreign), got),
			"a mover policy without the operator label must be left in place")
	})

	t.Run("owned policy is deleted", func(t *testing.T) {
		owned := ownedVolsyncMoverPolicy(namespace)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).Build()
		require.NoError(t, reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace))

		err := c.Get(ctx, client.ObjectKeyFromObject(owned), &networkingv1.NetworkPolicy{})
		assert.True(t, apierrors.IsNotFound(err),
			"an operator-managed mover policy is removed when VolSync is not installed")
	})

	t.Run("delete not found is ignored", func(t *testing.T) {
		owned := ownedVolsyncMoverPolicy(namespace)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(
				context.Context, client.WithWatch, client.Object, ...client.DeleteOption,
			) error {
				return apierrors.NewNotFound(schema.GroupResource{
					Group:    "networking.k8s.io",
					Resource: "networkpolicies",
				}, NetworkPolicyVolsyncMover)
			},
		}).Build()
		require.NoError(t, reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace),
			"a concurrent delete should not fail reconciliation")
	})

	t.Run("get error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithInterceptorFuncs(interceptor.Funcs{
			Get: func(
				context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption,
			) error {
				return errors.New("get failed")
			},
		}).Build()
		err := reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace)
		require.EqualError(t, err, "failed to get volsync-mover networkpolicy: get failed",
			"lookup failures must be returned so reconciliation retries")
	})

	t.Run("delete error", func(t *testing.T) {
		owned := ownedVolsyncMoverPolicy(namespace)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(
				context.Context, client.WithWatch, client.Object, ...client.DeleteOption,
			) error {
				return errors.New("delete failed")
			},
		}).Build()
		err := reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace)
		require.EqualError(t, err, "failed to delete volsync-mover networkpolicy: delete failed",
			"delete failures must be returned so reconciliation retries")
	})

	t.Run("delete uses ownership preconditions", func(t *testing.T) {
		owned := ownedVolsyncMoverPolicy(namespace)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(
				ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption,
			) error {
				do := &client.DeleteOptions{}
				do.ApplyOptions(opts)
				require.NotNil(t, do.Preconditions, "delete must bind the ownership-checked object")
				require.NotNil(t, do.Preconditions.UID)
				require.NotNil(t, do.Preconditions.ResourceVersion)
				assert.Equal(t, owned.UID, *do.Preconditions.UID)
				return c.Delete(ctx, obj, opts...)
			},
		}).Build()
		require.NoError(t, reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace))
	})

	t.Run("delete conflict is returned", func(t *testing.T) {
		owned := ownedVolsyncMoverPolicy(namespace)
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(owned).WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(
				context.Context, client.WithWatch, client.Object, ...client.DeleteOption,
			) error {
				return apierrors.NewConflict(schema.GroupResource{
					Group:    "networking.k8s.io",
					Resource: "networkpolicies",
				}, NetworkPolicyVolsyncMover, errors.New("replaced"))
			},
		}).Build()
		err := reconcilerWithClient(c).deleteVolsyncMoverPolicy(ctx, namespace)
		require.ErrorContains(t, err, "failed to delete volsync-mover networkpolicy")
		assert.True(t, apierrors.IsConflict(err),
			"a conflict must be returned so reconciliation repeats the ownership check")
	})
}

func TestReconcileVolsyncGate(t *testing.T) {
	ctx := context.Background()
	scheme := volsyncTestScheme(t)
	require.NoError(t, corev1.AddToScheme(scheme))
	mgh := &v1alpha4.MulticlusterGlobalHub{
		ObjectMeta: metav1.ObjectMeta{Name: "hub", Namespace: "gh"},
	}

	t.Run("discovery error", func(t *testing.T) {
		r := &NetworkPolicyReconciler{
			Manager: &clientManager{
				c: fake.NewClientBuilder().WithScheme(scheme).WithObjects(mgh).Build(),
			},
			kubeClient: &stubKubeClient{
				discovery: &stubDiscovery{
					restClient: newVolsyncRESTClient(http.StatusOK, errors.New("discovery unavailable")),
				},
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{})
		require.ErrorContains(t, err, "failed to check whether volsync is installed")
		require.ErrorContains(t, err, "discovery unavailable")
	})

	t.Run("delete error", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mgh, ownedVolsyncMoverPolicy("gh")).
			WithInterceptorFuncs(interceptor.Funcs{
				Get: func(
					ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption,
				) error {
					if key.Name == NetworkPolicyVolsyncMover {
						return errors.New("get failed")
					}
					return c.Get(ctx, key, obj, opts...)
				},
			}).Build()
		r := &NetworkPolicyReconciler{
			Manager: &clientManager{c: c},
			kubeClient: &stubKubeClient{
				discovery: &stubDiscovery{restClient: newVolsyncRESTClient(http.StatusNotFound, nil)},
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{})
		require.EqualError(t, err, "failed to get volsync-mover networkpolicy: get failed")
	})

	t.Run("volsync installed", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mgh).Build()
		r := &NetworkPolicyReconciler{
			Manager: &clientManager{
				c: c,
				config: &rest.Config{
					Host: "https://example.invalid",
					Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
						return nil, errors.New("in-memory discovery transport")
					}),
				},
				scheme: scheme,
			},
			kubeClient: &stubKubeClient{
				discovery: &stubDiscovery{restClient: newVolsyncRESTClient(http.StatusOK, nil)},
			},
		}
		_, err := r.Reconcile(ctx, ctrl.Request{})
		require.ErrorContains(t, err, "in-memory discovery transport")
		assert.NotContains(t, err.Error(), "failed to check whether volsync is installed")
	})
}
