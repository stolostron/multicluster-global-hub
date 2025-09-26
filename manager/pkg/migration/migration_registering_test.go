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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestRegistering(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		setupState              func(migrationID string)
		expectedRequeue         bool
		expectedError           bool
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "Should skip registering if migration is being deleted",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					Finalizers:        []string{"test-finalizer"},
					UID:               types.UID("test-uid-1"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRegistering,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRegistering, // Should not change phase when being deleted
			expectedConditionStatus: "",                                 // No condition updates for deletion
			expectedConditionReason: "",
		},
		{
			name: "Should skip registering if already registered",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRegistering,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRegistering, // Should not change phase when already registered
			expectedConditionStatus: metav1.ConditionTrue,               // Condition should remain true
			expectedConditionReason: "",                                 // No reason change expected
		},
		{
			name: "Should skip if not in registering phase",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should not change phase when not in registering
			expectedConditionStatus: "",                               // No condition updates
			expectedConditionReason: "",
		},
		{
			name: "Should wait for target hub to complete registering",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-5"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRegistering,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseRegistering)
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseRegistering)
				// Don't set target hub finished to simulate waiting state
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRegistering, // Should remain in registering phase
			expectedConditionStatus: metav1.ConditionFalse,              // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,             // Should indicate waiting
		},
		{
			name: "Should handle error from target hub during registering",
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
					Phase: migrationv1alpha1.PhaseRegistering,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseRegistering)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseRegistering)
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseRegistering)
				SetErrorMessage(migrationID, "target-hub", migrationv1alpha1.PhaseRegistering, "registration failed")
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRollbacking, // Should move to rollbacking phase due to error
			expectedConditionStatus: metav1.ConditionFalse,              // Condition should be false due to error
			expectedConditionReason: ConditionReasonError,               // Should indicate error
		},
		{
			name: "Should complete when target hub finishes registering",
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
					Phase: migrationv1alpha1.PhaseRegistering,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// Mark source hub as started and finished so function proceeds to check target hub
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseRegistering)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseRegistering)
				SetStarted(migrationID, "target-hub", migrationv1alpha1.PhaseRegistering)
				SetFinished(migrationID, "target-hub", migrationv1alpha1.PhaseRegistering)
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseCleaning,  // Should move to cleaning phase
			expectedConditionStatus: metav1.ConditionTrue,             // Condition should be true
			expectedConditionReason: ConditionReasonClusterRegistered, // Should indicate registered
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

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.migration).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
				Scheme:   scheme,
			}

			requeue, err := controller.registering(context.TODO(), tt.migration)

			assert.Equal(t, tt.expectedRequeue, requeue, tt.name)
			if tt.expectedError {
				assert.Error(t, err, tt.name)
			} else {
				assert.NoError(t, err, tt.name)
			}

			// Verify the migration status after registering operation
			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase, "Expected phase should match")
			}

			// Verify the condition status if expected
			if tt.expectedConditionStatus != "" {
				registeredCondition := findRegisteringCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered)
				assert.NotNil(t, registeredCondition, "ClusterRegistered condition should exist")
				assert.Equal(t, tt.expectedConditionStatus, registeredCondition.Status, "Expected condition status should match")
			}

			// Verify the condition reason if expected
			if tt.expectedConditionReason != "" {
				registeredCondition := findRegisteringCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered)
				assert.NotNil(t, registeredCondition, "ClusterRegistered condition should exist")
				assert.Equal(t, tt.expectedConditionReason, registeredCondition.Reason, "Expected condition reason should match")
			}

			// Clean up after test
			RemoveMigrationStatus(string(tt.migration.GetUID()))
		})
	}
}

// findRegisteringCondition finds a specific condition in the conditions slice for registering tests
func findRegisteringCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
