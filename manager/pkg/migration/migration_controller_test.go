package migration

import (
	"context"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// MockProducer is a mock implementation of the transport.Producer for testing.
type MockProducer struct {
	SentEvents []cloudevents.Event
	SendError  error
}

// SendEvent mocks the sending of a cloud event.
func (m *MockProducer) SendEvent(ctx context.Context, event cloudevents.Event) error {
	if m.SendError != nil {
		return m.SendError
	}
	m.SentEvents = append(m.SentEvents, event)
	return nil
}

// Reconnect is a mock implementation of the Reconnect method.
func (m *MockProducer) Reconnect(config *transport.TransportInternalConfig, topic string) error {
	return nil
}

// findControllerCondition finds a specific condition in the conditions slice for controller tests
func findControllerCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

func TestUpdateStatusWithRetry(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		condition               metav1.Condition
		phase                   string
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "Should update condition and phase successfully",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhasePending,
				},
			},
			condition: metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeStarted,
				Status:  metav1.ConditionTrue,
				Reason:  ConditionReasonStarted,
				Message: "Migration instance is started",
			},
			phase:                   migrationv1alpha1.PhaseValidating,
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: ConditionReasonStarted,
		},
		{
			name: "Should update waiting condition",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-2",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-2"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseValidating,
				},
			},
			condition: metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeValidated,
				Status:  metav1.ConditionFalse,
				Reason:  ConditionReasonWaiting,
				Message: "Waiting for validation to complete",
			},
			phase:                   migrationv1alpha1.PhaseValidating,
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: ConditionReasonWaiting,
		},
		{
			name: "Should update rollback condition successfully",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-rollback",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-rollback"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
				},
			},
			condition: metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeRolledBack,
				Status:  metav1.ConditionTrue,
				Reason:  ConditionReasonResourceRolledBack,
				Message: "Migration rollback completed successfully",
			},
			phase:                   migrationv1alpha1.PhaseFailed,
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: ConditionReasonResourceRolledBack,
		},
		{
			name: "Should update rollback failure condition",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-rollback-fail",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid-rollback-fail"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
				},
			},
			condition: metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeRolledBack,
				Status:  metav1.ConditionFalse,
				Reason:  ConditionReasonRollbackFailed,
				Message: "Migration rollback failed due to timeout",
			},
			phase:                   migrationv1alpha1.PhaseFailed,
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: ConditionReasonRollbackFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.migration).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			ctx := context.TODO()
			err := controller.UpdateStatusWithRetry(ctx, tt.migration, tt.condition, tt.phase)
			assert.NoError(t, err)

			// Verify the migration status after update
			assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase, "Expected phase should match")

			// Verify the condition status and reason
			condition := findControllerCondition(tt.migration.Status.Conditions, tt.condition.Type)
			assert.NotNil(t, condition, "Condition should exist")
			assert.Equal(t, tt.expectedConditionStatus, condition.Status, "Expected condition status should match")
			assert.Equal(t, tt.expectedConditionReason, condition.Reason, "Expected condition reason should match")
			assert.Equal(t, tt.condition.Message, condition.Message, "Expected condition message should match")
		})
	}
}

func TestSelectAndPrepareMigration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migrations              []migrationv1alpha1.ManagedClusterMigration
		requestName             string
		expectedSelected        *string // nil if no migration should be selected
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
		expectedConditionType   string
	}{
		{
			name:        "Should initialize new migration to pending",
			requestName: "new-migration",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "new-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("new-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
						From:                    "source-hub",
						To:                      "target-hub",
						IncludedManagedClusters: []string{"cluster1"},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: "", // New migration with empty phase
					},
				},
			},
			expectedSelected:        stringPtr("new-migration"),
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: ConditionReasonStarted,
			expectedConditionType:   migrationv1alpha1.ConditionTypeStarted,
		},
		{
			name:        "Should select oldest pending migration",
			requestName: "older-migration",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "newer-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("newer-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhasePending,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "older-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("older-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhasePending,
					},
				},
			},
			expectedSelected:        stringPtr("older-migration"),
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: ConditionReasonStarted,
			expectedConditionType:   migrationv1alpha1.ConditionTypeStarted,
		},
		{
			name:        "Should skip completed and failed migrations",
			requestName: "pending-migration",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "completed-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("completed-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-2 * time.Hour)},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "failed-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("failed-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhaseFailed,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "pending-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("pending-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhasePending,
					},
				},
			},
			expectedSelected:        stringPtr("pending-migration"),
			expectedPhase:           migrationv1alpha1.PhaseValidating,
			expectedConditionStatus: metav1.ConditionTrue,
			expectedConditionReason: ConditionReasonStarted,
			expectedConditionType:   migrationv1alpha1.ConditionTypeStarted,
		},
		{
			name:        "Should handle deletion request",
			requestName: "deleting-migration",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "deleting-migration",
						Namespace:         utils.GetDefaultNamespace(),
						UID:               types.UID("deleting-uid"),
						CreationTimestamp: metav1.Time{Time: time.Now()},
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
						Finalizers:        []string{constants.ManagedClusterMigrationFinalizer},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhaseInitializing,
					},
				},
			},
			expectedSelected:        stringPtr("deleting-migration"),
			expectedPhase:           migrationv1alpha1.PhaseInitializing,
			expectedConditionStatus: "", // No condition change for deletion
			expectedConditionReason: "",
			expectedConditionType:   "",
		},
		{
			name:                    "Should return nil when no migrations available",
			requestName:             "non-existent",
			migrations:              []migrationv1alpha1.ManagedClusterMigration{},
			expectedSelected:        nil,
			expectedPhase:           "",
			expectedConditionStatus: "",
			expectedConditionReason: "",
			expectedConditionType:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to client.Object slice
			objects := make([]client.Object, len(tt.migrations))
			for i := range tt.migrations {
				objects[i] = &tt.migrations[i]
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			ctx := context.TODO()
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.requestName,
					Namespace: utils.GetDefaultNamespace(),
				},
			}

			selectedMigration, err := controller.selectAndPrepareMigration(ctx, req)
			assert.NoError(t, err)

			if tt.expectedSelected == nil {
				assert.Nil(t, selectedMigration, "Expected no migration to be selected")
			} else {
				assert.NotNil(t, selectedMigration, "Expected a migration to be selected")
				assert.Equal(t, *tt.expectedSelected, selectedMigration.Name, "Expected specific migration to be selected")

				if tt.expectedPhase != "" {
					assert.Equal(t, tt.expectedPhase, selectedMigration.Status.Phase, "Expected phase should match")
				}

				if tt.expectedConditionType != "" {
					condition := findControllerCondition(selectedMigration.Status.Conditions, tt.expectedConditionType)
					assert.NotNil(t, condition, "Expected condition should exist")
					if tt.expectedConditionStatus != "" {
						assert.Equal(t, tt.expectedConditionStatus, condition.Status, "Expected condition status should match")
					}
					if tt.expectedConditionReason != "" {
						assert.Equal(t, tt.expectedConditionReason, condition.Reason, "Expected condition reason should match")
					}
				}
			}
		})
	}
}

// Helper function for string pointer
func stringPtr(s string) *string {
	return &s
}

func TestSetupTimeoutsFromConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

	// Store original timeout values to restore after tests
	originalCleaningTimeout := cleaningTimeout
	originalMigrationStageTimeout := migrationStageTimeout
	originalRegisteringTimeout := registeringTimeout

	tests := []struct {
		name                       string
		migration                  *migrationv1alpha1.ManagedClusterMigration
		expectedCleaningTimeout    time.Duration
		expectedMigrationTimeout   time.Duration
		expectedRegisteringTimeout time.Duration
		expectError                bool
	}{
		{
			name: "Should set custom timeout from valid config",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{
						StageTimeout: &metav1.Duration{Duration: 15 * time.Minute},
					},
				},
			},
			expectedCleaningTimeout:    15 * time.Minute,
			expectedMigrationTimeout:   15 * time.Minute,
			expectedRegisteringTimeout: 15 * time.Minute,
			expectError:                false,
		},
		{
			name: "Should set custom timeout in seconds",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-seconds",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{
						StageTimeout: &metav1.Duration{Duration: 300 * time.Second},
					},
				},
			},
			expectedCleaningTimeout:    300 * time.Second,
			expectedMigrationTimeout:   300 * time.Second,
			expectedRegisteringTimeout: 300 * time.Second,
			expectError:                false,
		},
		{
			name: "Should set custom timeout in hours",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-hours",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{
						StageTimeout: &metav1.Duration{Duration: 2 * time.Hour},
					},
				},
			},
			expectedCleaningTimeout:    2 * time.Hour,
			expectedMigrationTimeout:   2 * time.Hour,
			expectedRegisteringTimeout: 2 * time.Hour,
			expectError:                false,
		},
		{
			name: "Should set custom timeout from duration",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-whitespace",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{
						StageTimeout: &metav1.Duration{Duration: 10 * time.Minute},
					},
				},
			},
			expectedCleaningTimeout:    10 * time.Minute,
			expectedMigrationTimeout:   10 * time.Minute,
			expectedRegisteringTimeout: 10 * time.Minute,
			expectError:                false,
		},
		{
			name: "Should keep defaults when no timeout specified",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-invalid",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{},
				},
			},
			expectedCleaningTimeout:    originalCleaningTimeout,
			expectedMigrationTimeout:   originalMigrationStageTimeout,
			expectedRegisteringTimeout: originalRegisteringTimeout,
			expectError:                false,
		},
		{
			name: "Should handle empty configs",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-nil-configs",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{},
				},
			},
			expectedCleaningTimeout:    originalCleaningTimeout,
			expectedMigrationTimeout:   originalMigrationStageTimeout,
			expectedRegisteringTimeout: originalRegisteringTimeout,
			expectError:                false,
		},
		{
			name: "Should handle empty timeout config",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-no-timeout",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: &migrationv1alpha1.ConfigMeta{},
				},
			},
			expectedCleaningTimeout:    originalCleaningTimeout,
			expectedMigrationTimeout:   originalMigrationStageTimeout,
			expectedRegisteringTimeout: originalRegisteringTimeout,
			expectError:                false,
		},
		{
			name: "Should handle nil SupportedConfigs",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration-nil-configs",
					Namespace: utils.GetDefaultNamespace(),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					SupportedConfigs: nil,
				},
			},
			expectedCleaningTimeout:    originalCleaningTimeout,
			expectedMigrationTimeout:   originalMigrationStageTimeout,
			expectedRegisteringTimeout: originalRegisteringTimeout,
			expectError:                false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset timeout values before each test
			cleaningTimeout = originalCleaningTimeout
			migrationStageTimeout = originalMigrationStageTimeout
			registeringTimeout = originalRegisteringTimeout

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.migration).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
			}

			err := controller.setupTimeoutsFromConfig(tt.migration)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			// Verify timeout values were set correctly
			assert.Equal(t, tt.expectedCleaningTimeout, cleaningTimeout, "cleaningTimeout should match expected value")
			assert.Equal(t, tt.expectedMigrationTimeout, migrationStageTimeout, "migrationStageTimeout should match expected value")
			assert.Equal(t, tt.expectedRegisteringTimeout, registeringTimeout, "registeringTimeout should match expected value")
		})
	}

	// Restore original timeout values after all tests
	t.Cleanup(func() {
		cleaningTimeout = originalCleaningTimeout
		migrationStageTimeout = originalMigrationStageTimeout
		registeringTimeout = originalRegisteringTimeout
	})
}
