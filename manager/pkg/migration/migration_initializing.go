package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"open-cluster-management.io/managed-serviceaccount/apis/authentication/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	ConditionMessageResourcePrepared = "All required resources have been prepared."

	ConditionReasonResourcePrepared = "ResourcePrepared"
	ConditionReasonSecretMissing    = "SecretMissing"
	ConditionReasonClusterNotSynced = "ClusterNotSynced"
	ConditionReasonTimeout          = "Timeout"
)

var migrationStageTimeout = 5 * time.Minute

// Initializing:
//  1. From Hub: sync the required clusters to databases
//  2. To Hub: create managedserviceaccount
func (m *ClusterMigrationController) initializing(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.Status.Phase == "" {
		mcm.Status.Phase = migrationv1alpha1.PhaseInitializing
		if err := m.Client.Status().Update(ctx, mcm); err != nil {
			return false, err
		}
	}

	// skip if the phase isn't Initializing and the MigrationResourceInitialized condition is True
	if mcm.Status.Phase != migrationv1alpha1.PhaseInitializing &&
		meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceInitialized) {
		return false, nil
	}

	// To Hub
	// check if the secret is created by managedserviceaccount, if not, ensure the managedserviceaccount
	if err := m.ensureManagedServiceAccount(ctx, mcm); err != nil {
		return false, err
	}

	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcm.Name,
			Namespace: mcm.Spec.To, // hub
		},
	}
	err := m.Client.Get(ctx, client.ObjectKeyFromObject(tokenSecret), tokenSecret)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, err
	} else if apierrors.IsNotFound(err) {
		err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationResourceInitialized,
			metav1.ConditionFalse, ConditionReasonSecretMissing,
			fmt.Sprintf("token secret(%s/%s) is not found!", tokenSecret.Namespace, tokenSecret.Name))
		if err != nil {
			return false, err
		}
		return true, nil
	}

	// check the migration clusters in the databases, if not, send it again
	leafHubToClusters, err := getLeafHubToClusters(mcm)
	if err != nil {
		return false, err
	}

	// From Hub
	// send the migration event to migration.from managed hub(s)
	notReadyClusters := []string{}
	db := database.GetGorm()
	for fromHubName, clusters := range leafHubToClusters {
		clusterResourcesSynced := true
		var initialized []models.ManagedClusterMigration
		if err = db.Where("from_hub = ? AND to_hub = ?", fromHubName, mcm.Spec.To).Find(&initialized).Error; err != nil {
			return false, err
		}
		initializedClusters := make([]string, len(initialized))
		for i, cluster := range initialized {
			initializedClusters[i] = cluster.ClusterName
		}
		// assert whether synced, if synced, change the cluster status into migrating
		for _, cluster := range clusters {
			if !utils.ContainsString(initializedClusters, cluster) {
				notReadyClusters = append(notReadyClusters, cluster)
				clusterResourcesSynced = false
			}
		}
		if !clusterResourcesSynced {
			err := m.specToFromHub(ctx, fromHubName, mcm.Spec.To, migrationv1alpha1.PhaseInitializing, clusters, nil, nil)
			if err != nil {
				return false, err
			}
		}
	}

	if len(notReadyClusters) > 0 {
		err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationResourceInitialized,
			metav1.ConditionFalse, ConditionReasonClusterNotSynced,
			fmt.Sprintf("clusters %v are not synced into db", notReadyClusters))
		if err != nil {
			return false, err
		}
		return true, nil
	}

	err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationResourceInitialized,
		metav1.ConditionTrue, ConditionReasonResourcePrepared, ConditionMessageResourcePrepared)
	if err != nil {
		return false, err
	}
	return false, nil
}

// update with conflict error, and also add timeout validating in the conditions
func (m *ClusterMigrationController) UpdateConditionWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	// timeout
	if status == metav1.ConditionFalse && time.Since(mcm.CreationTimestamp.Time) > migrationStageTimeout {
		reason = ConditionReasonTimeout
	}

	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Client.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		if err := m.UpdateCondition(ctx, mcm, conditionType, status, reason, message); err != nil {
			return err
		}
		return nil
	})
}

func (m *ClusterMigrationController) UpdateCondition(
	ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	conditionType string,
	status metav1.ConditionStatus,
	reason, message string,
) error {
	newCondition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	update := meta.SetStatusCondition(&mcm.Status.Conditions, newCondition)

	// Handle phase updates based on condition type and status
	if status == metav1.ConditionFalse {
		if reason == ConditionReasonTimeout && mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
			mcm.Status.Phase = migrationv1alpha1.PhaseFailed
			update = true
		}
	} else {
		switch conditionType {
		case migrationv1alpha1.MigrationResourceInitialized, migrationv1alpha1.MigrationClusterRegistered:
			if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating {
				mcm.Status.Phase = migrationv1alpha1.PhaseMigrating
				update = true
			}
		case migrationv1alpha1.MigrationResourceDeployed:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseCompleted
				update = true
			}
		}
	}

	// Update the resource in Kubernetes only if there was a change
	if update {
		return m.Status().Update(ctx, mcm)
	}
	return nil
}

// specToFromHub specifies the manager send the message into "From Hub" via spec path(or topic)
func (m *ClusterMigrationController) specToFromHub(ctx context.Context, fromHub string, toHub string, stage string,
	managedClusters []string, klusterletConfig *klusterletv1alpha1.KlusterletConfig, bootstrapSecret *corev1.Secret,
) error {
	managedClusterMigrationFromEvent := &bundleevent.ManagedClusterMigrationFromEvent{
		Stage:            stage,
		ToHub:            toHub,
		ManagedClusters:  managedClusters,
		BootstrapSecret:  bootstrapSecret,
		KlusterletConfig: klusterletConfig,
	}

	payloadBytes, err := json.Marshal(managedClusterMigrationFromEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal managed cluster migration from event(%v) - %w",
			managedClusterMigrationFromEvent, err)
	}

	eventType := constants.CloudEventTypeMigrationFrom
	evt := utils.ToCloudEvent(eventType, constants.CloudEventSourceGlobalHub, fromHub, payloadBytes)
	if err := m.Producer.SendEvent(ctx, evt); err != nil {
		return fmt.Errorf("failed to sync managedclustermigration event(%s) from source(%s) to destination(%s) - %w",
			eventType, constants.CloudEventSourceGlobalHub, fromHub, err)
	}
	return nil
}

func getLeafHubToClusters(mcm *migrationv1alpha1.ManagedClusterMigration) (map[string][]string, error) {
	db := database.GetGorm()
	managedClusterMap := make(map[string][]string)
	if mcm.Spec.From != "" {
		managedClusterMap[mcm.Spec.From] = mcm.Spec.IncludedManagedClusters
		return managedClusterMap, nil
	}

	rows, err := db.Raw(`SELECT leaf_hub_name, cluster_name FROM status.managed_clusters
			WHERE cluster_name IN (?)`,
		mcm.Spec.IncludedManagedClusters).Rows()
	if err != nil {
		return nil, fmt.Errorf("failed to get leaf hub name and managed clusters - %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var leafHubName, managedClusterName string
		if err := rows.Scan(&leafHubName, &managedClusterName); err != nil {
			return nil, fmt.Errorf("failed to scan leaf hub name and managed cluster name - %w", err)
		}
		// If the cluser is synced into the to hub, ignore it.
		if leafHubName == mcm.Spec.To {
			continue
		}
		managedClusterMap[leafHubName] = append(managedClusterMap[leafHubName], managedClusterName)
	}
	return managedClusterMap, nil
}

func (m *ClusterMigrationController) ensureManagedServiceAccount(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	// create a desired msa
	desiredMSA := &v1beta1.ManagedServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcm.GetName(),
			Namespace: mcm.Spec.To,
			Labels: map[string]string{
				"owner": strings.ToLower(constants.ManagedClusterMigrationKind),
			},
		},
		Spec: v1beta1.ManagedServiceAccountSpec{
			Rotation: v1beta1.ManagedServiceAccountRotation{
				Enabled: true,
				Validity: metav1.Duration{
					Duration: 86400 * time.Hour,
				},
			},
		},
	}

	existingMSA := &v1beta1.ManagedServiceAccount{}
	err := m.Client.Get(ctx, types.NamespacedName{
		Name:      mcm.GetName(),
		Namespace: mcm.Spec.To,
	}, existingMSA)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return m.Client.Create(ctx, desiredMSA)
		}
		return err
	}
	return nil
}
