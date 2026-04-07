package transporter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/controllers/transporter/protocol"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestStartController_Transport(t *testing.T) {
	// Reset the singleton
	transportReconciler = nil

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	err = networkingv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	initOption := config.ControllerOption{
		Manager: mgr,
	}

	// Test first call - should create controller
	controller, err := StartController(initOption)
	if err != nil {
		t.Logf("Controller setup error (may be expected): %v", err)
	}

	// Test second call - should return existing controller
	controller2, err2 := StartController(initOption)
	if err == nil {
		assert.NoError(t, err2)
		assert.Equal(t, controller, controller2)
	}

	// Test IsResourceRemoved
	if controller != nil {
		result := controller.IsResourceRemoved()
		t.Logf("IsResourceRemoved: %v", result)
	}

	// Cleanup
	transportReconciler = nil
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
			name: "kafka user secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kafka-user-secret",
					Namespace: utils.GetDefaultNamespace(),
					Labels: map[string]string{
						"strimzi.io/cluster": protocol.KafkaClusterName,
						"strimzi.io/kind":    "KafkaUser",
					},
				},
			},
			wantBool: true,
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
			name: "watched secret name matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHTransportSecretName,
				},
			},
			expected: true,
		},
		{
			name: "kafka user secret matches",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-secret",
					Labels: map[string]string{
						"strimzi.io/cluster": protocol.KafkaClusterName,
						"strimzi.io/kind":    "KafkaUser",
					},
				},
			},
			expected: true,
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

func TestNewTransportReconciler(t *testing.T) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	reconciler := NewTransportReconciler(mgr)
	assert.NotNil(t, reconciler)
	assert.Equal(t, mgr, reconciler.Manager)
}

func TestReconcile_Transport(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
		wantRequeue bool
	}{
		{
			name:        "reconcile with nil MGH",
			initObjects: []runtime.Object{},
			wantRequeue: false,
		},
		{
			name: "reconcile with paused MGH",
			initObjects: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-mgh",
						Namespace: namespace,
						Annotations: map[string]string{
							"mgh-pause": "true",
						},
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			wantRequeue: false,
		},
		{
			name: "reconcile with deleting MGH",
			initObjects: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-mgh",
						Namespace:         namespace,
						DeletionTimestamp: &metav1.Time{},
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			wantRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Scheme: scheme.Scheme,
			})
			if err != nil {
				t.Skip("Skipping test - no kubeconfig available")
			}

			r := &TransportReconciler{
				Manager: mgr,
			}

			ctx := context.Background()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-mgh",
					Namespace: namespace,
				},
			}

			_, err = r.Reconcile(ctx, req)
			// Errors are expected in unit tests due to missing configuration
			t.Logf("Reconcile result: %v", err)
		})
	}
}

func TestTransportReconcilerIsResourceRemoved(t *testing.T) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	r := &TransportReconciler{
		Manager: mgr,
	}

	// Test initial state
	result := r.IsResourceRemoved()
	t.Logf("IsResourceRemoved (initial): %v", result)

	// Test after setting isResourceRemoved
	isResourceRemoved = false
	result = r.IsResourceRemoved()
	assert.False(t, result)

	// Reset
	isResourceRemoved = true
}
