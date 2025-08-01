/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestInitializing(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = v1beta1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		existingObjects         []client.Object
		setupState              func(migrationID string)
		expectedRequeue         bool
		expectedError           bool
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "Should skip initializing if migration is being deleted",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid-1"),
					Finalizers:        []string{"test-finalizer"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should not change phase when being deleted
			expectedConditionStatus: "",                                  // No condition updates for deletion
			expectedConditionReason: "",
		},
		{
			name: "Should skip initializing if already initialized",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should not change phase when already initialized
			expectedConditionStatus: metav1.ConditionTrue,                // Condition should remain true
			expectedConditionReason: "",                                  // No reason change expected
		},
		{
			name: "Should skip if not in initializing phase",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseValidating, // Should not change phase when not in initializing
			expectedConditionStatus: "",                                // No condition updates
			expectedConditionReason: "",
		},
		{
			name: "Should wait for token secret to be created",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-4"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			existingObjects: []client.Object{},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetSourceClusters(migrationID, "source-hub", []string{"cluster1"})
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should remain in initializing phase
			expectedConditionStatus: metav1.ConditionFalse,               // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,              // Should indicate waiting
		},
		{
			name: "Should wait for target hub to complete initialization",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-5"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-migration",
						Namespace: "target-hub",
					},
					Data: map[string][]byte{
						"token":  []byte("test-token"),
						"ca.crt": []byte("test-ca"),
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
					Spec: clusterv1.ManagedClusterSpec{
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{URL: "https://test-hub.com"},
						},
					},
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetSourceClusters(migrationID, "source-hub", []string{"cluster-1"})
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				// Don't set target hub finished to simulate waiting state
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should remain in initializing phase
			expectedConditionStatus: metav1.ConditionFalse,               // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,              // Should indicate waiting
		},
		{
			name: "Should wait for source hub to complete initialization",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-6"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-migration",
						Namespace: "target-hub",
					},
					Data: map[string][]byte{
						"token":  []byte("test-token"),
						"ca.crt": []byte("test-ca"),
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
					Spec: clusterv1.ManagedClusterSpec{
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{URL: "https://test-hub.com"},
						},
					},
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetSourceClusters(migrationID, "source-hub", []string{"cluster-1"})
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				SetFinished(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				// Don't set source hub finished to simulate waiting state
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseInitializing, // Should remain in initializing phase
			expectedConditionStatus: metav1.ConditionFalse,               // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,              // Should indicate waiting
		},
		{
			name: "Should complete when both hubs finish initialization",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-7"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-migration",
						Namespace: "target-hub",
					},
					Data: map[string][]byte{
						"token":  []byte("test-token"),
						"ca.crt": []byte("test-ca"),
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
					Spec: clusterv1.ManagedClusterSpec{
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{URL: "https://test-hub.com"},
						},
					},
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetSourceClusters(migrationID, "source-hub", []string{"cluster-1"})
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				SetFinished(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseInitializing)
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying,   // Should move to deploying phase
			expectedConditionStatus: metav1.ConditionTrue,               // Condition should be true
			expectedConditionReason: ConditionReasonResourceInitialized, // Should indicate initialized
		},
		{
			name: "Should handle error from target hub during initialization",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-8"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			existingObjects: []client.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-migration",
						Namespace: "target-hub",
					},
					Data: map[string][]byte{
						"token":  []byte("test-token"),
						"ca.crt": []byte("test-ca"),
					},
				},
				&clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "target-hub",
					},
					Spec: clusterv1.ManagedClusterSpec{
						ManagedClusterClientConfigs: []clusterv1.ClientConfig{
							{URL: "https://test-hub.com"},
						},
					},
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetSourceClusters(migrationID, "source-hub", []string{"cluster-1"})
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing)
				SetErrorMessage(migrationID, "target-hub", migrationv1alpha1.PhaseInitializing, "initialization failed")
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRollbacking, // Should move to rollbacking phase due to error
			expectedConditionStatus: metav1.ConditionFalse,              // Condition should be false due to error
			expectedConditionReason: ConditionReasonError,               // Should indicate error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up any existing migration status
			RemoveMigrationStatus(string(tt.migration.GetUID()))

			// Setup state if provided
			if tt.setupState != nil {
				tt.setupState(string(tt.migration.GetUID()))
			}

			allObjects := append(tt.existingObjects, tt.migration)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(allObjects...).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			requeue, err := controller.initializing(context.TODO(), tt.migration)

			assert.Equal(t, tt.expectedRequeue, requeue, tt.name)
			if tt.expectedError {
				assert.Error(t, err, tt.name)
			} else {
				assert.NoError(t, err, tt.name)
			}

			// Verify the migration status after initializing operation
			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase, "Expected phase should match")
			}

			// Verify the condition status if expected
			if tt.expectedConditionStatus != "" {
				initializedCondition := findInitializingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
				assert.NotNil(t, initializedCondition, "ResourceInitialized condition should exist")
				assert.Equal(t, tt.expectedConditionStatus, initializedCondition.Status, "Expected condition status should match")
			}

			// Verify the condition reason if expected
			if tt.expectedConditionReason != "" {
				initializedCondition := findInitializingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeInitialized)
				assert.NotNil(t, initializedCondition, "ResourceInitialized condition should exist")
				assert.Equal(t, tt.expectedConditionReason, initializedCondition.Reason, "Expected condition reason should match")
			}

			// Clean up after test
			RemoveMigrationStatus(string(tt.migration.GetUID()))
		})
	}
}

// findInitializingCondition finds a specific condition in the conditions slice for initializing tests
func findInitializingCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
