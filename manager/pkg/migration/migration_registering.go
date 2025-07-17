package migration

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	ConditionReasonClusterRegistered = "ClusterRegistered"
)

// Migrating - registering:
//  1. Source Hub: set the bootstrap secret for the migrating clusters, change hubAccpetClient to trigger registering
//     Related Issue: https://issues.redhat.com/browse/ACM-15758
//  2. Destination Hub: report the status of migrated managed cluster
//  3. Global Hub: update the CR status
func (m *ClusterMigrationController) registering(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.DeletionTimestamp != nil {
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeRegistered) ||
		mcm.Status.Phase != migrationv1alpha1.PhaseRegistering {
		return false, nil
	}

	log.Info("migration registering")

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeRegistered,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: "Waiting for the managed clusters to be registered into the target hub",
	}
	nextPhase := migrationv1alpha1.PhaseRegistering

	defer func() {
		if condition.Reason == ConditionReasonWaiting && m.isReachedTimeout(ctx, mcm, registeringTimeout) {
			condition.Reason = ConditionReasonTimeout
			condition.Message = fmt.Sprintf("[Timeout] %s", condition.Message)
			nextPhase = migrationv1alpha1.PhaseCleaning
		}

		if condition.Reason == ConditionReasonError {
			nextPhase = migrationv1alpha1.PhaseCleaning
		}

		err := m.UpdateStatusWithRetry(ctx, mcm, condition, nextPhase)
		if err != nil {
			log.Errorf("failed to update the %s condition: %v", condition.Type, err)
		}
	}()

	sourceHubToClusters := GetSourceClusters(string(mcm.GetUID()))
	if sourceHubToClusters == nil {
		condition.Message = fmt.Sprintf("not initialized the source clusters for migrationId: %s", string(mcm.GetUID()))
		condition.Reason = ConditionReasonError
		return false, nil
	}

	for fromHub := range sourceHubToClusters {
		if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering) {
			log.Infof("migration registering: %s", fromHub)
			// notify the source hub to start registering
			err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseRegistering,
				sourceHubToClusters[fromHub], nil, nil)
			if err != nil {
				condition.Message = err.Error()
				condition.Reason = ConditionReasonError
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
		err := m.sendEventToDestinationHub(ctx, mcm, migrationv1alpha1.PhaseRegistering, allClusters)
		if err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	}

	errMessage := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	if errMessage != "" {
		condition.Message = fmt.Sprintf("registering to hub %s error: %s", mcm.Spec.To, errMessage)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering) {
		condition.Message = fmt.Sprintf("waiting for managed clusters to register to the target hub %s", mcm.Spec.To)
		return true, nil
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonClusterRegistered
	condition.Message = "All migrated clusters have been successfully registered"
	nextPhase = migrationv1alpha1.PhaseCleaning

	return false, nil
}
