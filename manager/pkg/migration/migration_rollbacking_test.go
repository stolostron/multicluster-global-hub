package migration

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// go test -timeout 30s -run ^TestRollbacking$ ./manager/pkg/migration
func TestRollbacking(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)
	_ = addonv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		initialStates           map[string]map[string]bool // hub -> stage -> started/finished
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
		expectedConditionType   string
		expectedRequeue         bool
		expectedError           bool
		setupSourceClusters     bool
	}{
		{
			name: "should skip rollback when not in rollbacking phase",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseInitializing,
				},
			},
			expectedRequeue: false,
			expectedError:   false,
		},
		{
			name: "should skip rollback when already rolled back",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeRolledBack,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedRequeue: false,
			expectedError:   false,
		},
		{
			name: "should initiate rollback successfully",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:               migrationv1alpha1.ConditionTypeStarted,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			setupSourceClusters:     true,
			expectedRequeue:         true,
			expectedError:           false,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: ConditionReasonWaiting,
			expectedConditionType:   migrationv1alpha1.ConditionTypeRolledBack,
		},
		{
			name: "should complete rollback successfully",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					From: "source-hub",
					To:   "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:               migrationv1alpha1.ConditionTypeStarted,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			setupSourceClusters: true,
			initialStates: map[string]map[string]bool{
				"source-hub": {
					migrationv1alpha1.PhaseRollbacking: true, // finished
				},
				"target-hub": {
					migrationv1alpha1.PhaseRollbacking: true, // finished
				},
			},
			expectedRequeue:         false, // Should not requeue when rollback completes
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseFailed,     // Should move to failed phase after rollback
			expectedConditionStatus: metav1.ConditionTrue,              // Condition should be true (completed)
			expectedConditionReason: ConditionReasonResourceRolledBack, // Should indicate rollback completed
			expectedConditionType:   migrationv1alpha1.ConditionTypeRolledBack,
		},
		{
			name: "should handle timeout in rollback",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
					To: "target-hub",
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:               migrationv1alpha1.ConditionTypeStarted,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
						},
						{
							Type:               migrationv1alpha1.ConditionTypeRolledBack,
							Status:             metav1.ConditionFalse,
							Reason:             ConditionReasonWaiting,
							Message:            "waiting for source hub  to complete Rollbacking stage rollback",
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)}, // Simulate timeout
						},
					},
				},
			},
			setupSourceClusters: true,
			initialStates: map[string]map[string]bool{
				"source-hub-1": {
					migrationv1alpha1.PhaseRollbacking + "_started": true, // started but not finished
				},
			},
			expectedRequeue:         true,
			expectedError:           false,
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionStatus: metav1.ConditionFalse,
			expectedConditionReason: ConditionReasonTimeout,
			expectedConditionType:   migrationv1alpha1.ConditionTypeRolledBack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize migration status for this test, and simulate the rollbacking clusters
			AddMigrationStatus(string(tt.migration.UID))
			SetClusterList(string(tt.migration.UID), []string{"cluster1", "cluster2"})

			// Create necessary objects for the test
			objects := []client.Object{tt.migration}

			// Add target hub cluster if needed
			if tt.migration.Spec.To != "" {
				targetHubCluster := &clusterv1.ManagedCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: tt.migration.Spec.To,
					},
				}
				objects = append(objects, targetHubCluster)

				// Add ManagedServiceAccount addon for tests that need to send rollback events
				managedServiceAccountAddon := &addonv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "managed-serviceaccount",
						Namespace: tt.migration.Spec.To,
					},
					Spec: addonv1alpha1.ManagedClusterAddOnSpec{
						InstallNamespace: "test-install-namespace",
					},
					Status: addonv1alpha1.ManagedClusterAddOnStatus{
						Namespace: "test-status-namespace",
					},
				}
				objects = append(objects, managedServiceAccountAddon)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&migrationv1alpha1.ManagedClusterMigration{}).
				Build()

			controller := &ClusterMigrationController{
				Client:   fakeClient,
				Producer: &MockProducer{},
				Scheme:   scheme,
			}

			// Set up timeout configuration for non-timeout tests
			if tt.name != "should handle timeout in rollback" {
				// Create a migration with long timeout for non-timeout tests
				testMigration := tt.migration.DeepCopy()
				testMigration.Spec.SupportedConfigs = &migrationv1alpha1.ConfigMeta{
					StageTimeout: &metav1.Duration{Duration: 30 * time.Minute},
				}
				err := controller.SetupMigrationStageTimeout(testMigration)
				if err != nil {
					t.Fatalf("Failed to setup timeouts: %v", err)
				}
			} else {
				// For timeout test, use a very short timeout to ensure timeout occurs
				testMigration := tt.migration.DeepCopy()
				testMigration.Spec.SupportedConfigs = &migrationv1alpha1.ConfigMeta{
					StageTimeout: &metav1.Duration{Duration: 2 * time.Minute}, // 2 min stage = 4 min rollback timeout
				}
				err := controller.SetupMigrationStageTimeout(testMigration)
				if err != nil {
					t.Fatalf("Failed to setup timeouts: %v", err)
				}
			}

			// Setup initial states for migration tracking
			if tt.initialStates != nil {
				for hub, states := range tt.initialStates {
					for state, finished := range states {
						if finished {
							SetFinished(string(tt.migration.UID), hub, state)
						} else {
							SetStarted(string(tt.migration.UID), hub, state)
						}
					}
				}
			}

			ctx := context.TODO()
			requeue, err := controller.rollbacking(ctx, tt.migration)

			// Verify results
			if tt.expectedError {
				fmt.Println("expected error", err)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedRequeue, requeue)

			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase)
			}

			if tt.expectedConditionType != "" {
				condition := findControllerCondition(tt.migration.Status.Conditions, tt.expectedConditionType)
				utils.PrettyPrint(tt.migration.Status)
				assert.NotNil(t, condition, fmt.Sprintf("Expected condition should exist: %s", tt.expectedConditionType))
				if tt.expectedConditionStatus != "" {
					assert.Equal(t, tt.expectedConditionStatus, condition.Status)
				}
				if tt.expectedConditionReason != "" {
					assert.Equal(t, tt.expectedConditionReason, condition.Reason, condition.Message)
				}
			}

			// Clean up migration status after test
			RemoveMigrationStatus(string(tt.migration.UID))
		})
	}
}

func TestDetermineFailedStage(t *testing.T) {
	tests := []struct {
		name          string
		migration     *migrationv1alpha1.ManagedClusterMigration
		expectedStage string
	}{
		{
			name: "should determine failed initializing stage",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expectedStage: migrationv1alpha1.PhaseInitializing,
		},
		{
			name: "should determine failed deploying stage",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   migrationv1alpha1.ConditionTypeDeployed,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expectedStage: migrationv1alpha1.PhaseDeploying,
		},
		{
			name: "should determine failed registering stage",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   migrationv1alpha1.ConditionTypeDeployed,
							Status: metav1.ConditionTrue,
						},
						{
							Type:   migrationv1alpha1.ConditionTypeRegistered,
							Status: metav1.ConditionFalse,
						},
					},
				},
			},
			expectedStage: migrationv1alpha1.PhaseRegistering,
		},
		{
			name: "should default to current phase when no failed condition found",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:   migrationv1alpha1.ConditionTypeInitialized,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			expectedStage: migrationv1alpha1.PhaseRollbacking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &ClusterMigrationController{}
			ctx := context.TODO()

			stage := controller.determineFailedStage(ctx, tt.migration)
			assert.Equal(t, tt.expectedStage, stage)
		})
	}
}

func TestHandleRollbackStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name                    string
		migration               *migrationv1alpha1.ManagedClusterMigration
		condition               *metav1.Condition
		nextPhase               *string
		expectedPhase           string
		expectedConditionReason string
		simulateTimeout         bool
	}{
		{
			name: "should handle successful rollback",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:               migrationv1alpha1.ConditionTypeStarted,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now()},
						},
					},
				},
			},
			condition: &metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeRolledBack,
				Status:  metav1.ConditionTrue,
				Reason:  ConditionReasonResourceRolledBack,
				Message: "Rollback completed successfully",
			},
			nextPhase:               stringPtr(migrationv1alpha1.PhaseFailed),
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionReason: ConditionReasonResourceRolledBack,
		},
		{
			name: "should handle rollback timeout",
			migration: &migrationv1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-migration",
					Namespace: utils.GetDefaultNamespace(),
					UID:       types.UID("test-uid"),
				},
				Status: migrationv1alpha1.ManagedClusterMigrationStatus{
					Phase: migrationv1alpha1.PhaseRollbacking,
					Conditions: []metav1.Condition{
						{
							Type:               migrationv1alpha1.ConditionTypeStarted,
							Status:             metav1.ConditionTrue,
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)},
						},
						{
							Type:               migrationv1alpha1.ConditionTypeRolledBack,
							Status:             metav1.ConditionFalse,
							Reason:             ConditionReasonWaiting,
							Message:            "Waiting for rollback to complete",
							LastTransitionTime: metav1.Time{Time: time.Now().Add(-6 * time.Minute)}, // Simulate timeout
						},
					},
				},
			},
			condition: &metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeRolledBack,
				Status:  metav1.ConditionFalse,
				Reason:  ConditionReasonWaiting,
				Message: "Waiting for rollback to complete",
			},
			nextPhase:               stringPtr(migrationv1alpha1.PhaseRollbacking),
			simulateTimeout:         true,
			expectedPhase:           migrationv1alpha1.PhaseFailed,
			expectedConditionReason: ConditionReasonTimeout,
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
				Scheme:   scheme,
			}

			// Set up timeout configuration for timeout tests
			if tt.simulateTimeout {
				// Use short timeout for timeout test
				testMigration := tt.migration.DeepCopy()
				testMigration.Spec.SupportedConfigs = &migrationv1alpha1.ConfigMeta{
					StageTimeout: &metav1.Duration{Duration: 2 * time.Minute}, // 2 min stage = 4 min rollback timeout
				}
				err := controller.SetupMigrationStageTimeout(testMigration)
				if err != nil {
					t.Fatalf("Failed to setup timeouts: %v", err)
				}
			} else {
				// Use default timeouts for non-timeout tests
				err := controller.SetupMigrationStageTimeout(tt.migration)
				if err != nil {
					t.Fatalf("Failed to setup timeouts: %v", err)
				}
			}

			ctx := context.TODO()
			waitingHub := "test-hub"
			failedStage := "Initializing"
			var calculatedSuccessClusters []string
			controller.handleRollbackStatus(ctx, tt.migration, tt.condition, tt.nextPhase, &waitingHub, failedStage, &calculatedSuccessClusters)

			// Verify the results
			assert.Equal(t, tt.expectedPhase, tt.migration.Status.Phase)

			condition := findControllerCondition(tt.migration.Status.Conditions, tt.condition.Type)
			assert.NotNil(t, condition)
			assert.Equal(t, tt.expectedConditionReason, condition.Reason)

			if tt.simulateTimeout {
				assert.Contains(t, condition.Message, "Timeout")
			}
		})
	}
}
