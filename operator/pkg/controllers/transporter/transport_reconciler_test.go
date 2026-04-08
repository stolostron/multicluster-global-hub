package transporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// Predicate Tests - These test critical watch logic for Kafka-related resources

const testSecretName = "some-secret"

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
			// Test CreateFunc
			createEvent := event.CreateEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, secretPred.Create(createEvent))

			// Test UpdateFunc
			updateEvent := event.UpdateEvent{
				ObjectNew: tt.obj,
			}
			assert.Equal(t, tt.wantBool, secretPred.Update(updateEvent))

			// Test DeleteFunc
			deleteEvent := event.DeleteEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, secretPred.Delete(deleteEvent))
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
			name: "kafka network policy in any namespace should match (only checks name)",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      protocol.KafkaClusterName,
					Namespace: "other-namespace",
				},
			},
			wantBool: true,
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
			// Test CreateFunc
			createEvent := event.CreateEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, networkPolicyPred.Create(createEvent))

			// Test UpdateFunc
			updateEvent := event.UpdateEvent{
				ObjectNew: tt.obj,
			}
			assert.Equal(t, tt.wantBool, networkPolicyPred.Update(updateEvent))

			// Test DeleteFunc
			deleteEvent := event.DeleteEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, networkPolicyPred.Delete(deleteEvent))
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
					Name: tt.objName,
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
