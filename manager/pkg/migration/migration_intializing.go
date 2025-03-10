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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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
)

var initializingTimeout = 5 * time.Minute

// Initializing:
//  1. From Hub: sync the required clusters to databases
//  2. To Hub: create managedserviceaccount
func (m *ClusterMigrationController) initializing(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	if mcm.Status.Phase == "" {
		mcm.Status.Phase = migrationv1alpha1.PhaseInitializing
		m.Client.Status().Update(ctx, mcm)
	}

	// only handle the intializing stage
	if mcm.Status.Phase != migrationv1alpha1.PhaseInitializing && mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
		return false, nil
	}

	secretIsReady := false
	notReadyClusters := []string{}

	// To Hub
	// check if the secret is created by managedserviceaccount, if not, ensure the managedserviceaccount
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
		secretIsReady = true
		if err := m.ensureManagedServiceAccount(ctx, mcm); err != nil {
			return false, err
		}
	}

	// check the migration clusters in the databases, if not, send it again
	leafHubToClusters, err := getLeafHubToClusters(mcm)
	if err != nil {
		return false, err
	}

	// From Hub
	// send the migration event to migration.from managed hub(s)
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
		// assert whether synced
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

	if (!secretIsReady || len(notReadyClusters) > 0) && time.Since(mcm.CreationTimestamp.Time) < initializingTimeout {
		log.Info("Waiting for migration resource to be ready")
		return true, nil
	}

	conditionMessage := ConditionMessageResourcePrepared
	conditionReason := ConditionReasonResourcePrepared
	conditionStatus := metav1.ConditionTrue
	if !secretIsReady {
		conditionMessage = fmt.Sprintf("token secret is not found %s/%s", tokenSecret.Namespace, tokenSecret.Name)
		conditionReason = ConditionReasonSecretMissing
		conditionStatus = metav1.ConditionFalse
	}
	if len(notReadyClusters) > 0 {
		conditionMessage = fmt.Sprintf("clusters are synced into db %v", notReadyClusters)
		conditionReason = ConditionReasonClusterNotSynced
		conditionStatus = metav1.ConditionFalse
	}
	err = m.UpdateCondition(ctx, mcm, migrationv1alpha1.MigrationResourceInitialized, conditionStatus,
		conditionReason, conditionMessage)
	if err != nil {
		return false, fmt.Errorf("failed to update migration condition: %w", err)
	}
	return false, nil
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

	requireUpdate := false
	exist := false
	// Check if the condition already exists and update if necessary
	for i, cond := range mcm.Status.Conditions {
		if cond.Type == conditionType {
			exist = true
			if cond.Status != status || cond.Reason != reason || cond.Message != message {
				requireUpdate = true
				mcm.Status.Conditions[i] = newCondition
			}
			break
		}
	}

	// Append new condition if not found
	if !exist {
		mcm.Status.Conditions = append(mcm.Status.Conditions, newCondition)
		requireUpdate = true
	}

	// Handle phase updates based on condition type and status
	switch {
	case status == metav1.ConditionFalse:
		if mcm.Status.Phase != migrationv1alpha1.PhaseFailed {
			mcm.Status.Phase = migrationv1alpha1.PhaseFailed
			requireUpdate = true
		}
	case status == metav1.ConditionTrue:
		switch conditionType {
		case migrationv1alpha1.MigrationResourceInitialized, migrationv1alpha1.MigrationClusterRegistered:
			if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating {
				mcm.Status.Phase = migrationv1alpha1.PhaseMigrating
				requireUpdate = true
			}
		case migrationv1alpha1.MigrationResourceDeployed:
			if mcm.Status.Phase != migrationv1alpha1.PhaseCompleted {
				mcm.Status.Phase = migrationv1alpha1.PhaseCompleted
				requireUpdate = true
			}
		}
	}

	// Update the resource in Kubernetes only if there was a change
	if requireUpdate {
		return m.Status().Update(ctx, mcm)
	}
	return nil
}

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
