package migration

import (
	"context"
	"sort"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	ConditionReasonStarted = "InstanceStarted"
	ConditionReasonWaiting = "Waiting"
)

// selectAndPrepareMigration selects the oldest migration and updates its status to start the migration.
// It returns the current migration object. It returns nil if no migration is in progress.
func (m *ClusterMigrationController) selectAndPrepareMigration(ctx context.Context,
	req ctrl.Request,
) (*migrationv1alpha1.ManagedClusterMigration, error) {
	migrationList := &migrationv1alpha1.ManagedClusterMigrationList{}
	if err := m.List(ctx, migrationList); err != nil {
		return nil, err
	}

	// Initialize the status of all new migrations to Pending
	for i := range migrationList.Items {
		migration := &migrationList.Items[i]
		if migration.Status.Phase == "" {
			err := m.UpdateStatusWithRetry(ctx, migration, metav1.Condition{
				Type:    migrationv1alpha1.ConditionTypeStarted,
				Status:  metav1.ConditionFalse,
				Reason:  ConditionReasonWaiting,
				Message: "Waiting for the migration to start",
			}, migrationv1alpha1.PhasePending)
			if err != nil {
				log.Errorf("failed to update migration to pending: %v", err)
				return nil, err
			}
		}
	}

	// Sort by creation timestamp to find the oldest one
	sort.Slice(migrationList.Items, func(i, j int) bool {
		return migrationList.Items[i].CreationTimestamp.Before(&migrationList.Items[j].CreationTimestamp)
	})

	var nextMigration *migrationv1alpha1.ManagedClusterMigration
	for i := range migrationList.Items {
		migration := &migrationList.Items[i]
		if migration.DeletionTimestamp != nil && req.Name == migration.Name {
			return migration, nil // Deleting migration should be processed
		}
		phase := migration.Status.Phase
		if phase != migrationv1alpha1.PhaseCompleted && phase != migrationv1alpha1.PhaseFailed {
			if nextMigration == nil {
				nextMigration = migration
			}
		}
	}

	// select the instance, and initialize it if it's not validated yet
	if nextMigration != nil &&
		meta.FindStatusCondition(nextMigration.Status.Conditions, migrationv1alpha1.ConditionTypeValidated) == nil {
		if err := m.UpdateStatusWithRetry(ctx, nextMigration, metav1.Condition{
			Type:    migrationv1alpha1.ConditionTypeStarted,
			Status:  metav1.ConditionTrue,
			Reason:  ConditionReasonStarted,
			Message: "Migration instance is started",
		}, migrationv1alpha1.PhaseValidating); err != nil {
			log.Errorf("failed to update migration to started: %v", err)
			return nil, err
		}
	}

	if nextMigration != nil {
		log.Infof("selected migration: %s (phase: %s)", nextMigration.Name, nextMigration.Status.Phase)
	} else {
		log.Infof("no migration selected")
	}
	return nextMigration, nil
}

// update with conflict error, and also add timeout validating in the conditions
func (m *ClusterMigrationController) UpdateStatusWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition metav1.Condition,
	phase string,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		condition.LastTransitionTime = metav1.NewTime(time.Now())

		if meta.SetStatusCondition(&mcm.Status.Conditions, condition) || mcm.Status.Phase != phase {
			mcm.Status.Phase = phase
			log.Infof("update condition %s(%s): %s, phase: %s", condition.Type, condition.Reason, condition.Message, phase)
			return m.Status().Update(ctx, mcm)
		}
		return nil
	})
}
