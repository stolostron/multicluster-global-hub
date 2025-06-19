package migration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

func TestGetCurrentMigration(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = migrationv1alpha1.AddToScheme(scheme)

	tests := []struct {
		name          string
		migrations    []migrationv1alpha1.ManagedClusterMigration
		expectedName  string
		preExec       func()
		expectedError bool
	}{
		{
			name:          "Should return nil when no migrations exist",
			migrations:    []migrationv1alpha1.ManagedClusterMigration{},
			expectedName:  "",
			expectedError: false,
		},
		{
			name: "Should skip the completed migration",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "migration1"},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhaseCompleted,
					},
				},
			},
			expectedName:  "",
			expectedError: false,
		},
		{
			name: "Should skip the failed migration since it was completed",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "migration2",
						UID:  "migration2",
					},
					Spec: migrationv1alpha1.ManagedClusterMigrationSpec{
						From:                    "hub1",
						To:                      "hub2",
						IncludedManagedClusters: []string{"cluster1"},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhaseFailed,
						Conditions: []metav1.Condition{
							{
								Type:    migrationv1alpha1.ConditionTypeCleaned,
								Status:  metav1.ConditionTrue,
								Reason:  "ResourceCleaned",
								Message: "Resources have been cleaned from the hub clusters",
							},
						},
					},
				},
			},
			expectedName:  "",
			expectedError: false,
			preExec: func() {
				AddMigrationStatus("migration2")
				SetFinished("migration2", "hub1", v1alpha1.PhaseCleaning)
				SetFinished("migration2", "hub2", v1alpha1.PhaseCleaning)
			},
		},
		{
			name: "Should return migration with finalizer",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "migration1",
						Finalizers: []string{constants.ManagedClusterMigrationFinalizer},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "migration2"},
				},
			},
			expectedName:  "migration1",
			expectedError: false,
		},
		{
			name: "Should return oldest migration when no finalizers exist",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "migration1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "migration2",
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedName:  "migration1",
			expectedError: false,
		},
		{
			name: "Should return oldest migration when no finalizers exist, and it's pending",
			migrations: []migrationv1alpha1.ManagedClusterMigration{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "migration1",
						CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					},
					Status: migrationv1alpha1.ManagedClusterMigrationStatus{
						Phase: migrationv1alpha1.PhasePending,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "migration2",
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
				},
			},
			expectedName:  "migration1",
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.preExec != nil {
				tt.preExec()
			}
			migrationList := &migrationv1alpha1.ManagedClusterMigrationList{
				Items: tt.migrations,
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithLists(migrationList).
				Build()

			controller := &ClusterMigrationController{
				Client: fakeClient,
			}

			migration, err := controller.getCurrentMigration(context.TODO(), reconcile.Request{})

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedName == "" {
					assert.Nil(t, migration)
				} else {
					assert.Equal(t, tt.expectedName, migration.Name)
				}
			}
		})
	}
}
