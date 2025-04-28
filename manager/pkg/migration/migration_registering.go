package migration

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	conditionReasonClusterRegistering     = "ClusterRegistering"
	conditionReasonClusterNotRegistered   = "ClusterNotRegistered"
	conditionReasonClusterRegistered      = "ClusterRegistered"
	conditionReasonAddonConfigNotDeployed = "AddonConfigNotDeployed"
	conditionReasonAddonConfigDeployed    = "AddonConfigDeployed"
)

// Migrating - registering:
//  1. Source Hub: set the bootstrap secret for the migrating clusters, change hubAccpetClient to trigger registering
//     Related Issue: https://issues.redhat.com/browse/ACM-15758
//  2. Destination Hub: don't need to do anything
//  3. Global Hub: update the staging in database from MigrationResourceInitialized -> MigrationClusterRegistered
func (m *ClusterMigrationController) registering(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseMigrating {
		return false, nil
	}

	log.Info("migration registering")

	condType := migrationv1alpha1.ConditionTypeRegistered
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonClusterRegistered
	condMsg := "All migrated clusters registered"
	var err error

	defer func() {
		if err != nil {
			condMsg = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonClusterNotRegistered
		}
		log.Infof("registering condition %s(%s): %s", condType, condReason, condMsg)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMsg)
		if err != nil {
			log.Errorf("failed to update the %s condition: %v", condType, err)
		}
	}()

	sourceHubToClusters, err := getSourceClusters(mcm)
	if err != nil {
		return false, err
	}

	for fromHub := range sourceHubToClusters {
		if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering) {
			log.Infof("migration registering: %s", fromHub)
			// notify the source hub to start registering
			err = m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseRegistering,
				nil, nil)
			if err != nil {
				return false, err
			}
			SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering)
		}
	}
	return true, nil
}
