package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
)

// ConfigMap keys used to store cluster migration results
const (
	// allClustersConfigMapKey stores the complete list of clusters involved in the migration
	allClustersConfigMapKey = "all-clusters"
	// successClustersConfigMapKey stores clusters that successfully completed their migration phase
	successClustersConfigMapKey = "success"
	// failedClustersConfigMapKey stores clusters that failed during migration
	failedClustersConfigMapKey = "failure"
)

// storeClustersToConfigMap creates or updates a ConfigMap with cluster data for the migration.
// The ConfigMap is named with the migration name and has an owner reference to the migration object.
// The clustersMap parameter contains key-value pairs where keys represent cluster states
// (e.g., "successClusters", "failedClusters") and values are lists of cluster names.
func (m *ClusterMigrationController) storeClustersToConfigMap(ctx context.Context,
	migration *migrationv1alpha1.ManagedClusterMigration, clustersMap map[string][]string,
) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      migration.Name,
			Namespace: migration.Namespace,
		},
	}
	err := controllerutil.SetOwnerReference(migration, configMap, m.Scheme)
	if err != nil {
		return fmt.Errorf("failed to set owner reference for configmap: %s", configMap.Name)
	}

	configMapData := make(map[string]string)
	for key, clusters := range clustersMap {
		clustersData, err := json.Marshal(clusters)
		if err != nil {
			return fmt.Errorf("failed to marshal clusters data: %w", err)
		}
		configMapData[key] = string(clustersData)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		operation, err := controllerutil.CreateOrUpdate(ctx, m.Client, configMap, func() error {
			configMap.Data = configMapData
			return nil
		})
		log.Infof("save configmap for migration %s, operation: %v", configMap.Name, operation)
		return err
	})
	return err
}

// UpdateStatusWithRetry updates the migration status with retry logic to handle conflict errors.
// It sets the specified condition and phase on the migration object, automatically handling
// LastTransitionTime for changed conditions. If the phase is Failed, it also updates the
// failed clusters ConfigMap before returning.
func (m *ClusterMigrationController) UpdateStatusWithRetry(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
	condition metav1.Condition,
	phase string,
) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := m.Get(ctx, client.ObjectKeyFromObject(mcm), mcm); err != nil {
			return err
		}
		// Check if the condition exists and has changed
		existingCondition := meta.FindStatusCondition(mcm.Status.Conditions, condition.Type)
		conditionChanged := existingCondition == nil ||
			existingCondition.Status != condition.Status ||
			existingCondition.Reason != condition.Reason ||
			existingCondition.Message != condition.Message

		// Only set LastTransitionTime if condition has changed or doesn't exist
		if conditionChanged && condition.LastTransitionTime.IsZero() {
			condition.LastTransitionTime = metav1.NewTime(time.Now())
		}

		if meta.SetStatusCondition(&mcm.Status.Conditions, condition) || mcm.Status.Phase != phase {
			mcm.Status.Phase = phase

			log.Infof("update condition %s(%s): %s, phase: %s", condition.Type, condition.Reason, condition.Message, phase)
			if err := m.Status().Update(ctx, mcm); err != nil {
				return err
			}

			if mcm.Status.Phase == migrationv1alpha1.PhaseFailed {
				// save cluster list to configmap
				err := m.UpdateFailedClustersConfimap(ctx, mcm)
				if err != nil {
					return err
				}
			}
			return nil
		}
		return nil
	})
}

// UpdateFailedClustersConfimap updates the ConfigMap with failed cluster information.
// It calculates the failed clusters by subtracting successful clusters from all clusters
// and stores both successful and failed clusters in the ConfigMap.
func (m *ClusterMigrationController) UpdateFailedClustersConfimap(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	// Handle failed migration
	successClusters, err := m.getClustersFromExistingConfigMap(ctx, mcm.Name, mcm.Namespace, successClustersConfigMapKey)
	if err != nil {
		return err
	}
	allClusters := GetClusterList(string(mcm.UID))
	if len(successClusters) == 0 {
		if err := m.storeClustersToConfigMap(ctx, mcm,
			map[string][]string{
				// Store all failed clusters
				failedClustersConfigMapKey: allClusters,
			}); err != nil {
			return fmt.Errorf("failed to store clusters to ConfigMap: %w", err)
		}
		return nil
	}

	// failedClusters = allClusters - successClusters
	failedClusters := []string{}
	for _, cluster := range allClusters {
		exist := false
		for _, successCluster := range successClusters {
			if cluster == successCluster {
				exist = true
				break
			}
		}
		if !exist {
			failedClusters = append(failedClusters, cluster)
		}
	}

	if err := m.storeClustersToConfigMap(ctx, mcm,
		map[string][]string{
			successClustersConfigMapKey: successClusters,
			failedClustersConfigMapKey:  failedClusters,
		}); err != nil {
		return fmt.Errorf("failed to store clusters to ConfigMap: %w", err)
	}

	return nil
}

// UpdateSuccessClustersConfimap updates the ConfigMap with successful cluster information.
// It stores the provided list of successful clusters in the ConfigMap.
func (m *ClusterMigrationController) UpdateSuccessClustersConfimap(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration, successClusters []string,
) error {
	return m.storeClustersToConfigMap(ctx, mcm, map[string][]string{
		successClustersConfigMapKey: successClusters,
	})
}

// UpdateAllClustersConfimap updates the ConfigMap with all cluster information.
// Currently it only stores successful clusters, but the name suggests it should handle all clusters.
// This function appears to be a duplicate of UpdateSuccessClustersConfimap.
func (m *ClusterMigrationController) UpdateAllClustersConfimap(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration, successClusters []string,
) error {
	return m.storeClustersToConfigMap(ctx, mcm, map[string][]string{
		allClustersConfigMapKey: successClusters,
	})
}

func (m *ClusterMigrationController) setClusterList(
	ctx context.Context, mcm *migrationv1alpha1.ManagedClusterMigration,
) error {
	if len(mcm.Spec.IncludedManagedClusters) != 0 {
		SetClusterList(string(mcm.UID), mcm.Spec.IncludedManagedClusters)
		return nil
	}
	clusters, err := m.getClustersFromExistingConfigMap(ctx, mcm.Name, mcm.Namespace, allClustersConfigMapKey)
	if err != nil {
		return err
	}
	if len(clusters) > 0 {
		SetClusterList(string(mcm.UID), clusters)
	}
	return nil
}

// getClustersFromExistingConfigMap attempts to retrieve clusters from an existing ConfigMap
// Returns clusters slice, found boolean, and error
func (m *ClusterMigrationController) getClustersFromExistingConfigMap(ctx context.Context,
	configMapName, namespace string, key string,
) ([]string, error) {
	existingConfigMap := &corev1.ConfigMap{}
	err := m.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: namespace,
	}, existingConfigMap)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// ConfigMap exists, get clusters from it
	if clustersJSON, exists := existingConfigMap.Data[key]; exists {
		var clusters []string
		if err := json.Unmarshal([]byte(clustersJSON), &clusters); err != nil {
			return nil, fmt.Errorf("failed to unmarshal clusters from ConfigMap: %w", err)
		}
		log.Infof("retrieved clusters from existing ConfigMap %s/%s for migration %s",
			namespace, configMapName, configMapName)
		return clusters, nil
	}

	return nil, nil
}
