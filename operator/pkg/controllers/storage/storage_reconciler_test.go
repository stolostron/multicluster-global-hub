// Copyright (c) 2026 Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	assert.Equal(t, want, pred.Create(event.CreateEvent{Object: obj}),
		"predicate Create should return %v for watched resource events", want)
	assert.Equal(t, want, pred.Update(event.UpdateEvent{ObjectNew: obj}),
		"predicate Update should return %v for watched resource events", want)
	assert.Equal(t, want, pred.Delete(event.DeleteEvent{Object: obj}),
		"predicate Delete should return %v for watched resource events", want)
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
			assert.Equal(t, tt.wantCreate, configMapPredicate.Create(event.CreateEvent{Object: tt.obj}),
				"configMapPredicate Create should return %v for %q", tt.wantCreate, tt.name)
			assert.Equal(t, tt.wantUpdateDelete, configMapPredicate.Update(event.UpdateEvent{ObjectNew: tt.obj}),
				"configMapPredicate Update should return %v for %q", tt.wantUpdateDelete, tt.name)
			assert.Equal(t, tt.wantUpdateDelete, configMapPredicate.Delete(event.DeleteEvent{Object: tt.obj}),
				"configMapPredicate Delete should return %v for %q", tt.wantUpdateDelete, tt.name)
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
			assert.Equal(t, tt.wantCreate, secretPred.Create(event.CreateEvent{Object: tt.obj}),
				"secretPred Create should return %v for %q", tt.wantCreate, tt.name)
			assert.Equal(t, tt.wantUpdateDelete, secretPred.Update(event.UpdateEvent{ObjectNew: tt.obj}),
				"secretPred Update should return %v for %q", tt.wantUpdateDelete, tt.name)
			assert.Equal(t, tt.wantUpdateDelete, secretPred.Delete(event.DeleteEvent{Object: tt.obj}),
				"secretPred Delete should return %v for %q", tt.wantUpdateDelete, tt.name)
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
			assert.Equal(t, tt.wantType, condition.Type,
				"retention condition type should be %q for %q", tt.wantType, tt.name)
			assert.Equal(t, tt.wantStatus, string(condition.Status),
				"retention condition status should be %q for %q", tt.wantStatus, tt.name)
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
			assert.Equal(t, tt.length, len(password),
				"generated password length must match requested length %d", tt.length)

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
	assert.True(t, reconciler.IsResourceRemoved(), "storage reconciler should report resources as removed")
}

// TestReadonlyUsernameFromURI verifies readonly URI parse failures do not leak credentials.
func TestReadonlyUsernameFromURI(t *testing.T) {
	t.Run("valid uri", func(t *testing.T) {
		username, err := readonlyUsernameFromURI("postgresql://readonly:secret@postgres.example:5432/hoh")
		require.NoError(t, err, "valid readonly URI must parse")
		assert.Equal(t, "readonly", username, "readonly username must match the URI")
	})

	t.Run("malformed uri with percent-escaped password does not leak credentials", func(t *testing.T) {
		const sentinel = "secret-password"
		raw := "postgresql://readonly:" + sentinel + "%zz@postgres.example:5432/hoh"

		_, err := readonlyUsernameFromURI(raw)
		require.Error(t, err, "malformed readonly URI must be rejected")
		assert.Equal(t, errParseReadonlyUserURI, err.Error(),
			"parse errors must use a fixed message")
		assert.NotContains(t, err.Error(), raw,
			"parse errors must not echo the raw connection URI")
		assert.NotContains(t, err.Error(), "postgresql://",
			"parse errors must not echo the URI scheme")
		assert.NotContains(t, err.Error(), sentinel,
			"parse errors must not echo the postgres password")
	})

	t.Run("uri without userinfo returns empty username", func(t *testing.T) {
		username, err := readonlyUsernameFromURI("postgresql://postgres.example:5432/hoh")
		require.NoError(t, err, "readonly URI without userinfo must not fail database init")
		assert.Empty(t, username, "missing userinfo should skip privileges.sql username substitution")
	})

	t.Run("uri with empty username returns empty username", func(t *testing.T) {
		username, err := readonlyUsernameFromURI("postgresql://:secret@postgres.example:5432/hoh")
		require.NoError(t, err, "readonly URI with empty username must not fail database init")
		assert.Empty(t, username, "empty username should skip privileges.sql username substitution")
	})

	t.Run("invalid role name with SQL metacharacters is rejected", func(t *testing.T) {
		_, err := readonlyUsernameFromURI("postgresql://readonly%27;DROP--:secret@postgres.example:5432/hoh")
		require.Error(t, err, "readonly URI with unsafe role name must be rejected")
		assert.Equal(t, errInvalidPostgresRoleName, err.Error(),
			"unsafe role names must not reach privileges.sql substitution")
	})
}
