package migration

import (
	"context"
	"fmt"

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
//  2. Destination Hub: report the status of migrated managed cluster
//  3. Global Hub: update the CR status
func (m *ClusterMigrationController) registering(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseRegistering {
		return false, nil
	}

	log.Info("migration registering")

	condType := migrationv1alpha1.ConditionTypeRegistered
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonClusterRegistered
	condMessage := "All migrated clusters are registered"
	var err error

	defer func() {
		if err != nil {
			condMessage = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonClusterNotRegistered
		}
		log.Infof("registering condition %s(%s): %s", condType, condReason, condMessage)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMessage)
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
				sourceHubToClusters[fromHub], nil)
			if err != nil {
				return false, err
			}
			SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering)
		}
	}

	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering) {
		log.Infof("migration registering: %s", mcm.Spec.To)
		allClusters := []string{}
		for _, clusters := range sourceHubToClusters {
			allClusters = append(allClusters, clusters...)
		}
		// notify the target hub to start registering
		err = m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.PhaseRegistering, allClusters)
		if err != nil {
			return false, err
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	}

	errMessage := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	if errMessage != "" {
		err = fmt.Errorf("register to hub %s with error: %s", mcm.Spec.To, errMessage)
		return false, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering) {
		condMessage = fmt.Sprintf("the migrated clusters are registering to the target hub %s", mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		condReason = conditionReasonClusterRegistered
		return true, nil
	}

	return true, nil
}
