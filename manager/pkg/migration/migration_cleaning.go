package migration

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

const (
	conditionReasonResourceNotCleaned = "ResourceNotCleaned"
	conditionReasonResourceCleaned    = "ResourceCleaned"
)

// Completed:
// 1. Once the cluster registered into the destination hub, Clean up the resources in the source hub
// 2. Clean up the resource if the migration failed, to let it rollback
func (m *ClusterMigrationController) completed(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if !mcm.DeletionTimestamp.IsZero() {
		RemoveMigrationStatus(string(mcm.GetUID()))
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned) ||
		(mcm.Status.Phase != migrationv1alpha1.PhaseCleaning && mcm.Status.Phase != migrationv1alpha1.PhaseFailed) {
		return false, nil
	}

	log.Infof("migration start cleaning: %s - %s", mcm.Name, mcm.Status.Phase)

	condType := migrationv1alpha1.ConditionTypeCleaned
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonResourceCleaned
	condMsg := "Resources have been cleaned from the hub clusters"
	var err error

	defer func() {
		if err != nil {
			condMsg = err.Error()
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonResourceNotCleaned
		}
		log.Infof("cleaning condition %s(%s): %s", condType, condReason, condMsg)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMsg)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}
	}()

	// Deleting the ManagedServiceAccount will revoke the bootstrap kubeconfig secret of the migrated cluster.
	// Be cautious — this action may carry potential risks.
	if err := m.deleteManagedServiceAccount(ctx, mcm); err != nil {
		log.Errorf("failed to delete the managedServiceAccount: %s/%s", mcm.Spec.To, mcm.Name)
		return false, err
	}

	sourceHubClusters := GetSourceClusters(string(mcm.GetUID()))
	if sourceHubClusters == nil {
		return false, fmt.Errorf("Not initialized the source clusters for migrationId: %s", string(mcm.GetUID()))
	}

	// cleanup the source hub: cleaning or failed state
	bootstrapSecret := getBootstrapSecret(mcm.Spec.To, nil)
	for sourceHub, clusters := range sourceHubClusters {
		if !GetStarted(string(mcm.GetUID()), sourceHub, migrationv1alpha1.PhaseCleaning) {
			err = m.sendEventToSourceHub(ctx, sourceHub, mcm, migrationv1alpha1.PhaseCleaning, clusters, nil, bootstrapSecret)
			if err != nil {
				return false, err
			}
			SetStarted(string(mcm.GetUID()), sourceHub, migrationv1alpha1.PhaseCleaning)
		}
	}

	// cleanup the target hub: cleaning or failed state
	if !GetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning) {
		if err := m.sendEventToDestinationHub(ctx, mcm, mcm.Status.Phase, mcm.Spec.IncludedManagedClusters); err != nil {
			return false, err
		}
		SetStarted(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning)
	}

	// check error message
	errMsg := GetErrorMessage(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning)
	if errMsg != "" {
		err = fmt.Errorf("cleaning target hub %s with err :%s", mcm.Spec.To, errMsg) // the err will be updated into cr
		return true, nil
	}
	for fromHubName := range sourceHubClusters {
		errMsg := GetErrorMessage(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseCleaning)
		if errMsg != "" {
			err = fmt.Errorf("cleaning source hub %s with err :%s", fromHubName, errMsg)
			return true, nil
		}
	}

	// confirmation from hubs
	if !GetFinished(string(mcm.GetUID()), mcm.Spec.To, migrationv1alpha1.PhaseCleaning) {
		condMsg = fmt.Sprintf("The target hub %s is cleaning", mcm.Spec.To)
		condStatus = metav1.ConditionFalse
		return true, nil
	}

	for fromHubName := range sourceHubClusters {
		if !GetFinished(string(mcm.GetUID()), fromHubName, migrationv1alpha1.PhaseCleaning) {
			condMsg = fmt.Sprintf("The source hub %s is cleaning", fromHubName)
			condStatus = metav1.ConditionFalse
			return true, nil
		}
	}

	log.Info("migration cleaning finished")
	return false, nil
}

func (m *ClusterMigrationController) deleteManagedServiceAccount(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration,
) error {
	msa := &v1beta1.ManagedServiceAccount{}
	if err := m.Get(ctx, types.NamespacedName{
		Name:      migration.Name,
		Namespace: migration.Spec.To,
	}, msa); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return m.Delete(ctx, msa)
}
