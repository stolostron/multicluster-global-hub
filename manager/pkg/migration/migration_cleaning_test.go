package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func TestCompleted(t *testing.T) {
	tests := []struct {
		name                    string
		mcm                     *v1alpha1.ManagedClusterMigration
		existingObjects         []client.Object
		sourceClusters          map[string][]string
		started                 map[string]bool
		finished                map[string]bool
		errorMsgs               map[string]string
		wantRequeue             bool
		wantErr                 bool
		expectedPhase           string
		expectedConditionStatus metav1.ConditionStatus
		expectedConditionReason string
	}{
		{
			name: "deletion timestamp not zero",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					CreationTimestamp: metav1.Time{Time: time.Now()},
					Finalizers:        []string{"test-finalizer"},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
				},
			},
			wantRequeue:             false,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseCleaning, // Should not change phase when being deleted
			expectedConditionStatus: "",                     // No condition updates for deletion
			expectedConditionReason: "",
		},
		{
			name: "already cleaned",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
					Conditions: []metav1.Condition{
						{
							Type:   v1alpha1.ConditionTypeCleaned,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			wantRequeue:             false,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseCleaning, // Should not change phase when already cleaned
			expectedConditionStatus: metav1.ConditionTrue,   // Condition should remain true
			expectedConditionReason: "",                     // No reason change expected
		},
		{
			name: "phase not cleaning or failed",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseInitializing,
				},
			},
			wantRequeue:             false,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseInitializing, // Should not change phase when not in cleaning
			expectedConditionStatus: "",                         // No condition updates
			expectedConditionReason: "",
		},
		{
			name: "cleaning in progress",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					UID:               types.UID("test-uid"),
				},
				Spec: v1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "destination-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
				},
			},
			existingObjects: []client.Object{
				&v1beta1.ManagedServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-migration",
						Namespace: "destination-hub",
					},
				},
			},
			sourceClusters: map[string][]string{
				"source-hub": {"cluster1", "cluster2"},
			},
			started: map[string]bool{
				"source-hub":      true,
				"destination-hub": true,
			},
			finished: map[string]bool{
				"source-hub":      false,
				"destination-hub": true,
			},
			wantRequeue:             true,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseCleaning, // Should remain in cleaning phase
			expectedConditionStatus: metav1.ConditionFalse,  // Condition should be false (still waiting)
			expectedConditionReason: ConditionReasonWaiting, // Should indicate waiting
		},
		{
			name: "cleaning completed",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					CreationTimestamp: metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid"),
				},
				Spec: v1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "destination-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
				},
			},
			sourceClusters: map[string][]string{
				"source-hub": {"cluster1", "cluster2"},
			},
			started: map[string]bool{
				"source-hub":      true,
				"destination-hub": true,
			},
			finished: map[string]bool{
				"source-hub":      true,
				"destination-hub": true,
			},
			wantRequeue:             false,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseCompleted,        // Should complete when all hubs finished
			expectedConditionStatus: metav1.ConditionTrue,           // Condition should be true
			expectedConditionReason: ConditionReasonResourceCleaned, // Should indicate cleaned
		},
		{
			name: "cleaning with error from target hub",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					Namespace:         utils.GetDefaultNamespace(),
					CreationTimestamp: metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid"),
				},
				Spec: v1alpha1.ManagedClusterMigrationSpec{
					From:                    "source-hub",
					To:                      "destination-hub",
					IncludedManagedClusters: []string{"cluster1", "cluster2"},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
				},
			},
			sourceClusters: map[string][]string{
				"source-hub": {"cluster1", "cluster2"},
			},
			started: map[string]bool{
				"source-hub":      true,
				"destination-hub": true,
			},
			errorMsgs: map[string]string{
				"destination-hub": "cleanup failed on destination hub",
			},
			wantRequeue:             false,
			wantErr:                 false,
			expectedPhase:           v1alpha1.PhaseCompleted, // Should complete despite error (design: cleanup failure doesn't affect migration success)
			expectedConditionStatus: metav1.ConditionFalse,   // Condition should be false due to error
			expectedConditionReason: ConditionReasonError,    // Should indicate error
		},
	}

	for _, tt := range tests {
		scheme := runtime.NewScheme()

		_ = v1beta1.AddToScheme(scheme)
		_ = v1alpha1.AddToScheme(scheme)
		t.Run(tt.name, func(t *testing.T) {
			allObjects := append(tt.existingObjects, tt.mcm)
			ctrl := &ClusterMigrationController{
				Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(allObjects...).WithStatusSubresource(&v1alpha1.ManagedClusterMigration{}).Build(),
				Producer: &MockProducer{},
			}
			ctx := context.Background()

			// Setup test environment
			AddMigrationStatus(string(tt.mcm.GetUID()))

			if tt.started != nil {
				for hub, started := range tt.started {
					if started {
						SetStarted(string(tt.mcm.GetUID()), hub, v1alpha1.PhaseCleaning)
					}
				}
			}

			if tt.finished != nil {
				for hub, finished := range tt.finished {
					if finished {
						SetFinished(string(tt.mcm.GetUID()), hub, v1alpha1.PhaseCleaning)
					}
				}
			}

			if tt.errorMsgs != nil {
				for hub, msg := range tt.errorMsgs {
					SetErrorMessage(string(tt.mcm.GetUID()), hub, v1alpha1.PhaseCleaning, msg)
				}
			}

			requeue, err := ctrl.cleaning(ctx, tt.mcm)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequeue, requeue)

			// Verify the migration status after cleaning operation
			if tt.expectedPhase != "" {
				assert.Equal(t, tt.expectedPhase, tt.mcm.Status.Phase, "Expected phase should match")
			}

			// Verify the condition status if expected
			if tt.expectedConditionStatus != "" {
				cleanedCondition := findCondition(tt.mcm.Status.Conditions, v1alpha1.ConditionTypeCleaned)
				assert.NotNil(t, cleanedCondition, "ResourceCleaned condition should exist")
				assert.Equal(t, tt.expectedConditionStatus, cleanedCondition.Status, "Expected condition status should match")
			}

			// Verify the condition reason if expected
			if tt.expectedConditionReason != "" {
				cleanedCondition := findCondition(tt.mcm.Status.Conditions, v1alpha1.ConditionTypeCleaned)
				assert.NotNil(t, cleanedCondition, "ResourceCleaned condition should exist")
				assert.Equal(t, tt.expectedConditionReason, cleanedCondition.Reason, "Expected condition reason should match")
			}
		})
	}
}

// findCondition finds a specific condition in the conditions slice
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
