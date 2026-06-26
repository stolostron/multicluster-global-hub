package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/operator/api/operator/v1alpha4"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	commonutils "github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// Predicate Tests - These test critical watch logic that determines which resources trigger reconciliation

// assertPredicateAllEvents asserts Create, Update, and Delete predicate results for the same expected value.
func assertPredicateAllEvents(t *testing.T, pred predicate.Funcs, obj client.Object, want bool) {
	t.Helper()
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}), "CreateFunc")
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}), "UpdateFunc")
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}), "DeleteFunc")
}

func TestConfigMapPredicate(t *testing.T) {
	// configMapPredicate.CreateFunc only checks the watched-name set; owner label is only
	// evaluated in UpdateFunc/DeleteFunc (operator-created resources don't need Create triggers).
	tests := []struct {
		name             string
		obj              *corev1.ConfigMap
		wantCreate       bool
		wantUpdateDelete bool
	}{
		{
			name: "watched configmap should match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      BuiltinPostgresCAName,
					Namespace: commonutils.GetDefaultNamespace(),
				},
			},
			wantCreate:       true,
			wantUpdateDelete: true,
		},
		{
			name: "configmap with owner label should match on update/delete but not create",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "some-configmap",
					Namespace: commonutils.GetDefaultNamespace(),
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
			},
			wantCreate:       false, // CreateFunc only checks name, not owner label
			wantUpdateDelete: true,  // UpdateFunc/DeleteFunc also check owner label
		},
		{
			name: "other configmap should not match",
			obj: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-configmap",
					Namespace: commonutils.GetDefaultNamespace(),
				},
			},
			wantCreate:       false,
			wantUpdateDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCreate, configMapPredicate.Create(event.CreateEvent{Object: tt.obj}), "CreateFunc")
			assert.Equal(t, tt.wantUpdateDelete, configMapPredicate.Update(event.UpdateEvent{ObjectNew: tt.obj}), "UpdateFunc")
			assert.Equal(t, tt.wantUpdateDelete, configMapPredicate.Delete(event.DeleteEvent{Object: tt.obj}), "DeleteFunc")
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
			assertPredicateAllEvents(t, statefulSetPred, tt.obj, tt.wantBool)
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
		{
			name: "wrong namespace should not match",
			obj: &networkingv1.NetworkPolicy{
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
			assertPredicateAllEvents(t, networkPolicyPred, tt.obj, tt.wantBool)
		})
	}
}

func TestSecretPredicate_Storage(t *testing.T) {
	// secretPred.CreateFunc only checks the watched-name set; owner label is only evaluated in
	// UpdateFunc/DeleteFunc (operator-created secrets don't need Create triggers).
	tests := []struct {
		name             string
		obj              *corev1.Secret
		wantCreate       bool
		wantUpdateDelete bool
	}{
		{
			name: "watched secret should match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: constants.GHStorageSecretName,
				},
			},
			wantCreate:       true,
			wantUpdateDelete: true,
		},
		{
			name: "secret with owner label should match on update/delete but not create",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "some-secret",
					Labels: map[string]string{
						constants.GlobalHubOwnerLabelKey: constants.GHOperatorOwnerLabelVal,
					},
				},
			},
			wantCreate:       false, // CreateFunc only checks name, not owner label
			wantUpdateDelete: true,  // UpdateFunc/DeleteFunc also check owner label
		},
		{
			name: "other secret should not match",
			obj: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-secret",
				},
			},
			wantCreate:       false,
			wantUpdateDelete: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantCreate, secretPred.Create(event.CreateEvent{Object: tt.obj}), "CreateFunc")
			assert.Equal(t, tt.wantUpdateDelete, secretPred.Update(event.UpdateEvent{ObjectNew: tt.obj}), "UpdateFunc")
			assert.Equal(t, tt.wantUpdateDelete, secretPred.Delete(event.DeleteEvent{Object: tt.obj}), "DeleteFunc")
		})
	}
}

// Utility Function Tests - These test pure functions with business logic

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
		{
			name: "empty retention is invalid",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "",
						},
					},
				},
			},
			wantType:   config.CONDITION_TYPE_DATABASE,
			wantStatus: config.CONDITION_STATUS_FALSE,
		},
		{
			name: "retention with year format",
			mgh: &v1alpha4.MulticlusterGlobalHub{
				Spec: v1alpha4.MulticlusterGlobalHubSpec{
					DataLayerSpec: v1alpha4.DataLayerSpec{
						Postgres: v1alpha4.PostgresSpec{
							Retention: "1y",
						},
					},
				},
			},
			wantType:   config.CONDITION_TYPE_DATABASE,
			wantStatus: config.CONDITION_STATUS_TRUE,
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
		{
			name:   "generate 64 char password",
			length: 64,
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
					(char >= '0' && char <= '9'),
					"Password contains non-alphanumeric character: %c", char)
			}

			// Test that multiple calls generate different passwords (randomness check)
			password2 := generatePassword(tt.length)
			// It's extremely unlikely (but not impossible) that two random passwords are identical
			// This is a heuristic check
			if tt.length > 8 {
				assert.NotEqual(t, password, password2, "Generated passwords should be random")
			}
		})
	}
}

func TestIsResourceRemoved(t *testing.T) {
	reconciler := &StorageReconciler{}
	assert.True(t, reconciler.IsResourceRemoved())
}
