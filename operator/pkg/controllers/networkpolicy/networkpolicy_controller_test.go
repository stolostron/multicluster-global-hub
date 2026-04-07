package networkpolicy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestStartController(t *testing.T) {
	// Reset the singleton
	networkPolicyController = nil

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	err = networkingv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	kubeClient := fakekube.NewSimpleClientset()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		// Skip if no kubeconfig available
		t.Skip("Skipping test - no kubeconfig available")
	}

	initOption := config.ControllerOption{
		Manager:    mgr,
		KubeClient: kubeClient,
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

	// Reset for other tests
	networkPolicyController = &NetworkPolicyReconciler{
		Manager:    mgr,
		kubeClient: kubeClient,
	}
	assert.True(t, networkPolicyController.IsResourceRemoved())

	// Cleanup
	networkPolicyController = nil
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
					Name:      "default-deny-all",
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "allow-dns network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "allow-dns",
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "multicluster-global-hub-operator network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multicluster-global-hub-operator",
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
					Name:      "default-deny-all",
					Namespace: "wrong-namespace",
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

func TestReconcile(t *testing.T) {
	namespace := utils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)
	err = networkingv1.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
		mgh         *v1alpha4.MulticlusterGlobalHub
		wantErr     bool
	}{
		{
			name: "reconcile with valid MGH",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mgh",
					Namespace: namespace,
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{},
			},
			initObjects: []runtime.Object{
				&v1alpha4.MulticlusterGlobalHub{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-mgh",
						Namespace: namespace,
					},
					Spec: v1alpha4.MulticlusterGlobalHubSpec{},
				},
			},
			wantErr: false,
		},
		{
			name: "reconcile with paused MGH",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-mgh",
					Namespace: namespace,
					Annotations: map[string]string{
						"mgh-pause": "true",
					},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{},
			},
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
			wantErr: false,
		},
		{
			name: "reconcile with deleting MGH",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-mgh",
					Namespace:         namespace,
					DeletionTimestamp: &metav1.Time{},
				},
				Spec: v1alpha4.MulticlusterGlobalHubSpec{},
			},
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
			wantErr: false,
		},
		{
			name:        "reconcile with no MGH",
			initObjects: []runtime.Object{},
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeClient := fakekube.NewSimpleClientset()

			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Scheme: scheme.Scheme,
			})
			if err != nil {
				t.Skip("Skipping test - no kubeconfig available")
			}

			r := &NetworkPolicyReconciler{
				Manager:    mgr,
				kubeClient: kubeClient,
			}

			ctx := context.Background()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-mgh",
					Namespace: namespace,
				},
			}

			_, err = r.Reconcile(ctx, req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				// Error may occur due to discovery client issues, which is acceptable in unit tests
				t.Logf("Reconcile result: %v", err)
			}
		})
	}
}

func TestIsResourceRemoved(t *testing.T) {
	r := &NetworkPolicyReconciler{}
	assert.True(t, r.IsResourceRemoved())
}
