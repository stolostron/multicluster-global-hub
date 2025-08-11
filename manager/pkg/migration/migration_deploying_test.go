package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestDeploying(t *testing.T) {
	scheme := runtime.NewScheme()
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
			name: "Should skip deploying if migration is being deleted",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid-1"),
					Finalizers:        []string{"test-finalizer"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should not change phase when being deleted
			expectedConditionStatus: "",                               // No condition updates for deletion
			expectedConditionReason: "",
		},
		{
			name: "Should skip deploying if already deployed",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeDeployed,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should not change phase when already deployed
			expectedConditionStatus: metav1.ConditionTrue,             // Condition should remain true
			expectedConditionReason: "",                               // No reason change expected
		},
		{
			name: "Should wait when source clusters not initialized",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-3"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				// Don't set up any source clusters to simulate uninitialized state
			},
			expectedRequeue:         true, // Should requeue to wait for initialization
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should remain in deploying phase
			expectedConditionStatus: metav1.ConditionFalse,            // Condition should be false (waiting)
			expectedConditionReason: ConditionReasonWaiting,           // Should indicate waiting
		},
		{
			name: "Should wait for source hub to complete deploying",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-4"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// SetSourceClusters function no longer exists in current implementation
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				// Don't set finished to simulate waiting state
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should remain in deploying phase
			expectedConditionStatus: metav1.ConditionFalse,            // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,           // Should indicate waiting
		},
		{
			name: "Should wait for target hub to complete deploying",
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
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// SetSourceClusters function no longer exists in current implementation
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				// Don't set target hub finished to simulate waiting state
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseDeploying, // Should remain in deploying phase
			expectedConditionStatus: metav1.ConditionFalse,            // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting,           // Should indicate waiting
		},
		{
			name: "Should complete when both source and target hubs finished",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-6"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// SetSourceClusters function no longer exists in current implementation
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetFinished(migrationID, "target-hub", migrationv1alpha1.PhaseDeploying)
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRegistering, // Should move to registering phase
			expectedConditionStatus: metav1.ConditionTrue,               // Condition should be true
			expectedConditionReason: ConditionReasonResourcesDeployed,   // Should indicate deployed
		},
		{
			name: "Should handle error from source hub",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-7"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// SetSourceClusters function no longer exists in current implementation
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetErrorMessage(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying, "deployment failed")
			},
			expectedRequeue:         false,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseRollbacking, // Should move to rollbacking phase due to error
			expectedConditionStatus: metav1.ConditionFalse,              // Condition should be false due to error
			expectedConditionReason: ConditionReasonError,               // Should indicate error
		},
		{
			name: "Should handle error from target hub",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-8"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "target-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseDeploying,
				},
			},
			setupState: func(migrationID string) {
				AddMigrationStatus(migrationID)
				// SetSourceClusters function no longer exists in current implementation
				SetStarted(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetFinished(migrationID, "source-hub", migrationv1alpha1.PhaseDeploying)
				SetErrorMessage(migrationID, "target-hub", migrationv1alpha1.PhaseDeploying, "target deployment failed")
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

			allObjects := append([]client.Object{}, tt.migration)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(allObjects...).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			requeue, err := controller.deploying(context.TODO(), tt.migration)

			assert.Equal(t, tt.expectedRequeue, requeue, tt.name)
			if tt.expectedError {
				assert.Error(t, err, tt.name)
			} else {
				assert.NoError(t, err, tt.name)
			}

			// Verify the migration status after deploying operation
			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase, "Expected phase should match")
			}

			// Verify the condition status if expected
			if tt.expectedConditionStatus != "" {
				deployedCondition := findDeployingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed)
				assert.NotNil(t, deployedCondition, "ResourceDeployed condition should exist")
				assert.Equal(t, tt.expectedConditionStatus, deployedCondition.Status, "Expected condition status should match")
			}

			// Verify the condition reason if expected
			if tt.expectedConditionReason != "" {
				deployedCondition := findDeployingCondition(tt.migration.Status.Conditions, migrationv1alpha1.ConditionTypeDeployed)
				assert.NotNil(t, deployedCondition, "ResourceDeployed condition should exist")
				assert.Equal(t, tt.expectedConditionReason, deployedCondition.Reason, "Expected condition reason should match")
			}

			// Clean up after test
			RemoveMigrationStatus(string(tt.migration.GetUID()))
		})
	}
}

// findDeployingCondition finds a specific condition in the conditions slice for deploying tests
func findDeployingCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
