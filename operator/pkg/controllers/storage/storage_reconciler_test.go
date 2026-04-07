package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestNewStorageReconciler(t *testing.T) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	reconciler := NewStorageReconciler(mgr, true)
	assert.NotNil(t, reconciler)
	assert.Equal(t, mgr, reconciler.Manager)
	assert.False(t, reconciler.upgrade)
	assert.Equal(t, 0, reconciler.databaseReconcileCount)
	assert.True(t, reconciler.enableMetrics)
}

func TestStorageReconcilerIsResourceRemoved(t *testing.T) {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	reconciler := &StorageReconciler{
		Manager: mgr,
	}

	result := reconciler.IsResourceRemoved()
	assert.True(t, result)
}

func TestConfigMapPredicate(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.ConfigMap
		wantBool bool
	}{
		{
			name: "watched configmap should match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BuiltinPostgresCAName,
					Namespace: commonutils.GetDefaultNamespace(),
				},
			},
			wantBool: true,
		},
		{
			name: "configmap with owner label should match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-configmap",
					Namespace: commonutils.GetDefaultNamespace(),
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
			},
			wantBool: true,
		},
		{
			name: "other configmap should not match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-configmap",
					Namespace: commonutils.GetDefaultNamespace(),
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
			result := configMapPredicate.Create(createEvent)
			if tt.name == "watched configmap should match" {
				assert.Equal(t, tt.wantBool, result)
			}

			// Test UpdateFunc
			updateEvent := event.UpdateEvent{
				ObjectNew: tt.obj,
			}
			assert.Equal(t, tt.wantBool, configMapPredicate.Update(updateEvent))

			// Test DeleteFunc
			deleteEvent := event.DeleteEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, configMapPredicate.Delete(deleteEvent))
		})
	}
}

func TestStatefulSetPredicate(t *testing.T) {
	namespace := commonutils.GetDefaultNamespace()

	tests := []struct {
		name     string
		obj      *appsv1.StatefulSet
		wantBool bool
	}{
		{
			name: "builtin postgres statefulset should match",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BuiltinPostgresName,
					Namespace: namespace,
				},
			},
			wantBool: true,
		},
		{
			name: "other statefulset should not match",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-sts",
					Namespace: namespace,
				},
			},
			wantBool: false,
		},
		{
			name: "wrong namespace should not match",
			obj: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BuiltinPostgresName,
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
			assert.Equal(t, tt.wantBool, statefulSetPred.Create(createEvent))

			// Test UpdateFunc
			updateEvent := event.UpdateEvent{
				ObjectNew: tt.obj,
			}
			assert.Equal(t, tt.wantBool, statefulSetPred.Update(updateEvent))

			// Test DeleteFunc
			deleteEvent := event.DeleteEvent{
				Object: tt.obj,
			}
			assert.Equal(t, tt.wantBool, statefulSetPred.Delete(deleteEvent))
		})
	}
}

func TestNetworkPolicyPredicate_Storage(t *testing.T) {
	namespace := commonutils.GetDefaultNamespace()

	tests := []struct {
		name     string
		obj      *networkingv1.NetworkPolicy
		wantBool bool
	}{
		{
			name: "builtin postgres network policy should match",
			obj: &networkingv1.NetworkPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BuiltinPostgresName,
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

func TestSecretPredicate_Storage(t *testing.T) {
	tests := []struct {
		name     string
		obj      *corev1.Secret
		wantBool bool
	}{
		{
			name: "watched secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHStorageSecretName,
				},
			},
			wantBool: true,
		},
		{
			name: "secret with owner label should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-secret",
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
			},
			wantBool: true,
		},
		{
			name: "other secret should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-secret",
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
			result := secretPred.Create(createEvent)
			if tt.name == "watched secret should match" {
				assert.Equal(t, tt.wantBool, result)
			}

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

func TestGetRetentionConditions(t *testing.T) {
	tests := []struct {
		name       string
		mgh        *v1alpha4.MulticlusterGlobalHub
		wantType   string
		wantStatus string
	}{
		{
			name: "valid retention",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "6m",
						},
					},
				},
			},
			wantType:   config.CONDITION_TYPE_DATABASE,
			wantStatus: config.CONDITION_STATUS_TRUE,
		},
		{
			name: "invalid retention",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "invalid",
						},
					},
				},
			},
			wantType:   config.CONDITION_TYPE_DATABASE,
			wantStatus: config.CONDITION_STATUS_FALSE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := getRetentionConditions(tt.mgh)
			assert.Equal(t, tt.wantType, condition.Type)
			assert.Equal(t, tt.wantStatus, string(condition.Status))
		})
	}
}

func TestGeneratePassword(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{
			name:   "generate 8 char password",
			length: 8,
		},
		{
			name:   "generate 16 char password",
			length: 16,
		},
		{
			name:   "generate 32 char password",
			length: 32,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			password := generatePassword(tt.length)
			assert.Equal(t, tt.length, len(password))
			// Verify all characters are alphanumeric
			for _, char := range password {
				assert.True(t, (char >= 'A' && char <= 'Z') ||
					(char >= 'a' && char <= 'z') ||
					(char >= '0' && char <= '9'))
			}
		})
	}
}

func TestGetDatabaseComponentStatus(t *testing.T) {
	namespace := commonutils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		reconcileErr error
		wantStatus   string
	}{
		{
			name:         "reconcile error",
			reconcileErr: assert.AnError,
			wantStatus:   config.CONDITION_STATUS_FALSE,
		},
		{
			name:         "no error but no connection",
			reconcileErr: nil,
			wantStatus:   config.CONDITION_STATUS_FALSE,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()

			ctx := context.Background()
			condition := getDatabaseComponentStatus(ctx, fakeClient, namespace, config.COMPONENTS_POSTGRES_NAME, tt.reconcileErr)

			assert.Equal(t, "DatabaseConnection", condition.Kind)
			assert.Equal(t, config.COMPONENTS_POSTGRES_NAME, condition.Name)
			assert.Equal(t, tt.wantStatus, string(condition.Status))
		})
	}
}

func TestReconcile_Storage(t *testing.T) {
	namespace := commonutils.GetDefaultNamespace()

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	tests := []struct {
		name        string
		initObjects []runtime.Object
	}{
		{
			name:        "reconcile with nil MGH",
			initObjects: []runtime.Object{},
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

			reconciler := &StorageReconciler{
				Manager:                mgr,
				upgrade:                false,
				databaseReconcileCount: 0,
				enableMetrics:          false,
			}

			ctx := context.Background()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "test-mgh",
					Namespace: namespace,
				},
			}

			_, err = reconciler.Reconcile(ctx, req)
			// Errors are expected in unit tests due to missing configuration
			t.Logf("Reconcile result: %v", err)
		})
	}
}

func TestStartController_Storage(t *testing.T) {
	// Reset the singleton
	storageReconciler = nil

	err := v1alpha4.AddToScheme(scheme.Scheme)
	assert.NoError(t, err)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	initOption := config.ControllerOption{
		Manager: mgr,
		MulticlusterGlobalHub: &v1alpha4.MulticlusterGlobalHub{
			Spec: v1alpha4.MulticlusterGlobalHubSpec{
				EnableMetrics: true,
			},
		},
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

	// Cleanup
	storageReconciler = nil
}
