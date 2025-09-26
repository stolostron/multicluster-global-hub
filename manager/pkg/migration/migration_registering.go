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

	log.Info("migration %v registering", mcm.Name)

	condition := metav1.Condition{
		Type:    migrationv1alpha1.ConditionTypeRegistered,
		Status:  metav1.ConditionFalse,
		Reason:  ConditionReasonWaiting,
		Message: "Waiting for the managed clusters to be registered into the target hub",
	}
	nextPhase := migrationv1alpha1.PhaseRegistering

	defer m.handleStatusWithRollback(ctx, mcm, &condition, &nextPhase, getTimeout(migrationv1alpha1.PhaseRegistering))

	fromHub := mcm.Spec.From
	clusters := GetClusterList(string(mcm.UID))

	if !GetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering) {
		// notify the source hub to start registering
		log.Infof("sending registering event to source hub: %s, clusters: %v", fromHub, clusters)
		err := m.sendEventToSourceHub(ctx, fromHub, mcm, migrationv1alpha1.PhaseRegistering,
			clusters, nil, "")
		if err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		SetStarted(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering)
	}

	errMessage := GetErrorMessage(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering)
	if errMessage != "" {
		condition.Message = fmt.Sprintf("registering to hub %s error: %s", fromHub, errMessage)
		condition.Reason = ConditionReasonError
		return false, nil
	}

	if !GetFinished(string(mcm.GetUID()), fromHub, migrationv1alpha1.PhaseRegistering) {
		condition.Message = fmt.Sprintf("waiting for managed clusters to migrating from source hub %s", fromHub)
		return true, nil
	}

	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering) {
		// notify the target hub to start registering
		log.Infof("sending registering event to target hub: %s, clusters: %v", mcm.Spec.To, clusters)
		err := m.sendEventToTargetHub(ctx, mcm, migrationv1alpha1.PhaseRegistering, clusters, "")
		if err != nil {
			condition.Message = err.Error()
			condition.Reason = ConditionReasonError
			return false, err
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	}

	errMessage = GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	if errMessage != "" {
		m.handleErrorList(mcm, mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
		condition.Message = fmt.Sprintf("registering to hub %s error: %s", mcm.Spec.To, errMessage)
		condition.Reason = ConditionReasonError

		registeringReadyClusters := GetReadyClusters(string(mcm.UID), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
		if err := m.UpdateSuccessClustersToConfigMap(ctx, mcm, registeringReadyClusters); err != nil {
			log.Errorf("failed to store clusters to ConfigMap: %w", err)
			return false, err
		}
		return false, nil
	}

	// waiting the resources deployed confirmation
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseRegistering) {
		condition.Message = fmt.Sprintf("waiting for managed clusters to register to the target hub %s", mcm.Spec.To)
		return true, nil
	}

	registeringReadyClusters := GetReadyClusters(string(mcm.UID), mcm.Spec.To, migrationv1alpha1.PhaseRegistering)
	if err := m.UpdateSuccessClustersToConfigMap(ctx, mcm, registeringReadyClusters); err != nil {
		log.Errorf("failed to store clusters to ConfigMap: %w", err)
		return false, err
	}

	condition.Status = metav1.ConditionTrue
	condition.Reason = ConditionReasonClusterRegistered
	condition.Message = "All migrated clusters have been successfully registered"
	nextPhase = migrationv1alpha1.PhaseCleaning

	return false, nil
}
