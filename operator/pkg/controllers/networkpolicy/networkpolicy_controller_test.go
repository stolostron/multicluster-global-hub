package networkpolicy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

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
