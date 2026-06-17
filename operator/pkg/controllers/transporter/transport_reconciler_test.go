package transporter

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
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// Predicate Tests - These test critical watch logic for Kafka-related resources

const testSecretName = "some-secret"

// assertPredicateAllEvents asserts Create, Update, and Delete predicate results for the same expected value.
func assertPredicateAllEvents(t *testing.T, pred predicate.Funcs, obj client.Object, want bool) {
	t.Helper()
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}), "CreateFunc")
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}), "UpdateFunc")
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}), "DeleteFunc")
}

func TestSecretPredicate(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		wantBool bool
	}{
		{
			name: "transport secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.GHTransportSecretName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "kafka user secret with strimzi labels should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    StrimziKindUser,
					},
				},
			},
			wantBool: true,
		},
		{
			name: "kafka secret with wrong cluster name should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						StrimziClusterLabel: "wrong-cluster",
						StrimziKindLabel:    StrimziKindUser,
					},
				},
			},
			wantBool: false,
		},
		{
			name: "kafka secret with wrong kind should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    "Kafka",
					},
				},
			},
			wantBool: false,
		},
		{
			name: "other secret should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-secret",
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertPredicateAllEvents(t, secretPred, tt.obj, tt.wantBool)
		})
	}
}

func TestNetworkPolicyPredicate_Transport(t *testing.T) {
	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		wantBool bool
	}{
		{
			name: "kafka network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "kafka network policy in wrong namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: "other-namespace",
				},
			},
			wantBool: false,
		},
		{
			name: "other network policy should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-policy",
					Namespace: utils.GetDefaultNamespace(),
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

// Helper Function Tests - These test the predicate condition logic

func TestNetworkPolicyCond(t *testing.T) {
	tests := []struct {
		name     string
		objName  string
		expected bool
	}{
		{
			name:     "kafka cluster name matches",
			objName:  protocol.KafkaClusterName,
			expected: true,
		},
		{
			name:     "other name does not match",
			objName:  "other-name",
			expected: false,
		},
		{
			name:     "empty name does not match",
			objName:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tt.objName,
					Namespace: utils.GetDefaultNamespace(),
				},
			}
			result := networkPolicyCond(obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecretCond(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		expected bool
	}{
		{
			name: "transport secret name matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHTransportSecretName,
				},
			},
			expected: true,
		},
		{
			name: "kafka user secret with both labels matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
						StrimziKindLabel:    StrimziKindUser,
					},
				},
			},
			expected: true,
		},
		{
			name: "kafka secret with only cluster label does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						StrimziClusterLabel: protocol.KafkaClusterName,
					},
				},
			},
			expected: false,
		},
		{
			name: "kafka secret with only kind label does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: testSecretName,
					Labels: map[string]string{
						StrimziKindLabel: StrimziKindUser,
					},
				},
			},
			expected: false,
		},
		{
			name: "other secret does not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-secret",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := secretCond(tt.obj)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// State Management Tests

func TestTransportReconcilerIsResourceRemoved(t *testing.T) {
	r := &TransportReconciler{}

	// Test with default state (should be true initially)
	originalState := isResourceRemoved
	defer func() {
		isResourceRemoved = originalState
	}()

	// Test when isResourceRemoved is true
	isResourceRemoved = true
	result := r.IsResourceRemoved()
	assert.True(t, result)

	// Test when isResourceRemoved is false
	isResourceRemoved = false
	result = r.IsResourceRemoved()
	assert.False(t, result)
}

func TestStartControllerSingleton(t *testing.T) {
	// Save and restore the singleton so we don't affect other tests.
	original := transportReconciler
	t.Cleanup(func() { transportReconciler = original })

	existing := &TransportReconciler{}
	transportReconciler = existing

	// Second call must return the pre-existing instance without error.
	controller, err := StartController(config.ControllerOption{})
	assert.NoError(t, err)
	assert.Equal(t, existing, controller)
}

func TestNetworkPolicyCondCustomNamespace(t *testing.T) {
	// networkPolicyCond uses kafkaNetworkPolicyWatchNamespace when set;
	// verify it does NOT fall back to the default namespace.
	originalNS := kafkaNetworkPolicyWatchNamespace
	t.Cleanup(func() { kafkaNetworkPolicyWatchNamespace = originalNS })

	customNamespace := "custom-kafka-namespace"
	kafkaNetworkPolicyWatchNamespace = customNamespace

	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		expected bool
	}{
		{
			name: "kafka NP in custom namespace should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: customNamespace,
				},
			},
			expected: true,
		},
		{
			name: "kafka NP in default namespace should NOT match when custom namespace is set",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: utils.GetDefaultNamespace(),
				},
			},
			expected: false,
		},
		{
			name: "unknown NP in custom namespace should not match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-policy",
					Namespace: customNamespace,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, networkPolicyCond(tt.obj))
		})
	}
}
