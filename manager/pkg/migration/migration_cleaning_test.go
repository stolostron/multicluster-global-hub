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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

func TestCompleted(t *testing.T) {
	tests := []struct {
		name           string
		mcm            *v1alpha1.ManagedClusterMigration
		sourceClusters map[string][]string
		started        map[string]bool
		finished       map[string]bool
		errorMsgs      map[string]string
		wantRequeue    bool
		wantErr        bool
	}{
		{
			name: "deletion timestamp not zero",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: time.Now()},
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
			},
			wantRequeue: false,
			wantErr:     false,
		},
		{
			name: "already cleaned",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Conditions: []metav1.Condition{
						{
							Type:   v1alpha1.ConditionTypeCleaned,
							Status: metav1.ConditionTrue,
						},
					},
				},
			},
			wantRequeue: false,
			wantErr:     false,
		},
		{
			name: "phase not cleaning or failed",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseInitializing,
				},
			},
			wantRequeue: false,
			wantErr:     false,
		},
		{
			name: "source clusters not initialized",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					CreationTimestamp: metav1.Time{Time: time.Now()},
				},
				Status: v1alpha1.ManagedClusterMigrationStatus{
					Phase: v1alpha1.PhaseCleaning,
				},
			},
			sourceClusters: nil,
			wantRequeue:    false,
			wantErr:        true,
		},
		{
			name: "cleaning in progress",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					CreationTimestamp: metav1.Time{Time: time.Now().Add(-1 * time.Hour)},
					Name:              "test-migration",
					UID:               types.UID("test-uid"),
				},
				Spec: v1alpha1.ManagedClusterMigrationSpec{
					To: "destination-hub",
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
				"source-hub":      false,
				"destination-hub": true,
			},
			wantRequeue: false,
			wantErr:     true,
		},
		{
			name: "cleaning completed",
			mcm: &v1alpha1.ManagedClusterMigration{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-migration",
					CreationTimestamp: metav1.Time{Time: time.Now()},
					UID:               types.UID("test-uid"),
				},
				Spec: v1alpha1.ManagedClusterMigrationSpec{
					To: "destination-hub",
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
			wantRequeue: false,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		scheme := runtime.NewScheme()

		_ = v1beta1.AddToScheme(scheme)
		t.Run(tt.name, func(t *testing.T) {
			ctrl := &ClusterMigrationController{
				Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			}
			ctx := context.Background()

			// Setup test environment
			if tt.sourceClusters != nil {
				AddSourceClusters(string(tt.mcm.GetUID()), tt.sourceClusters)
			}

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

			requeue, err := ctrl.completed(ctx, tt.mcm)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantRequeue, requeue)
		})
	}
}
