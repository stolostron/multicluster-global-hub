package migration

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	conditionReasonResourceNotCleaned = "ResourceNotCleaned"
	conditionReasonResourceCleaned    = "ResourceCleaned"
	cleaningTimeout                   = 10 * time.Minute // Separate timeout for cleaning phase
)

// cleaning handles the cleanup phase of migration
// 1. Once the cluster registered into the destination hub, Clean up the resources in the source hub
// 2. Clean up the resource if the migration failed, to let it rollback
func (m *ClusterMigrationController) cleaning(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	// TODO: Deprecated: Use this only to add the final cleanup condition—intermediate states are not shown in conditions
	succeed := false
	if !mcm.DeletionTimestamp.IsZero() {
		RemoveMigrationStatus(string(mcm.GetUID()))
		return false, nil
	}

	if meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.ConditionTypeCleaned) ||
		(mcm.Status.Phase != migrationv1alpha1.PhaseCleaning && mcm.Status.Phase != migrationv1alpha1.PhaseFailed) {
		return false, nil
	}

	log.Infof("migration start cleaning: %s - %s", mcm.Name, mcm.Status.Phase)

	var err error
	var condStatus metav1.ConditionStatus
	var condReason string
	var condMsg string
	condType := migrationv1alpha1.ConditionTypeCleaned

	// Add cleaned condition only when clean finished.
	defer func() {
		// Successfully cleaned the resources from the hub clusters
		if err == nil && succeed {
			condStatus = metav1.ConditionTrue
			condReason = conditionReasonResourceCleaned
			condMsg = "Resources have been successfully cleaned up from the hub clusters"
		} else if time.Since(mcm.CreationTimestamp.Time) > cleaningTimeout {
			// If clean timeout, update the condition
			errMessage := "cleanup timeout. "
			if err != nil {
				errMessage = fmt.Sprintf("cleanup failed: %s", err.Error())
			}
			condMsg = fmt.Sprintf("%s You may need to manually remove the annotation (%s) from the managed clusters.",
				errMessage, constants.ManagedClusterMigrating)
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonResourceNotCleaned
		} else {
			// requeue the migration to retry cleaning
			return
		}

		log.Infof("cleaning condition %s(%s): %s", condType, condReason, condMsg)
		err = m.UpdateConditionWithRetry(ctx, mcm, condType, condStatus, condReason, condMsg, cleaningTimeout)
		if err != nil {
			log.Errorf("failed to update the condition %v", err)
		}

		// Remove finalizer only when cleanup is successful
		if condStatus == metav1.ConditionTrue {
			if controllerutil.ContainsFinalizer(mcm, constants.ManagedClusterMigrationFinalizer) {
				controllerutil.RemoveFinalizer(mcm, constants.ManagedClusterMigrationFinalizer)
				if updateErr := m.Update(ctx, mcm); updateErr != nil {
					log.Errorf("failed to remove finalizer: %v", updateErr)
				}
			}
		}
	}()

	sourceHubClusters := GetSourceClusters(string(mcm.GetUID()))
	if sourceHubClusters == nil {
		log.Infof("skipping cleanup: migration %q not initialized", mcm.Name)
		succeed = true
		return false, nil
	}

	// Deleting the ManagedServiceAccount will revoke the bootstrap kubeconfig secret of the migrated cluster.
	// Be cautious — this action may carry potential risks.
	if err := m.deleteManagedServiceAccount(ctx, mcm); err != nil {
		log.Errorf("failed to delete the managedServiceAccount: %s/%s", mcm.Spec.To, mcm.Name)
		return false, err
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
	succeed = true
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
