package migration

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	migrationv1alpha1 "github.com/stolostron/multicluster-global-hub/operator/api/migration/v1alpha1"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

const (
	conditionReasonClusterNotRegistered   = "ClusterNotRegistered"
	conditionReasonAddonConfigNotDeployed = "AddonConfigNotDeployed"
	conditionReasonClusterMigrated        = "ClusterMigrated"
)

// Migrating:
//  1. From Hub: register the cluster into To Hub
//  2. To Hub: deploy the addon config into the current Hub
func (m *ClusterMigrationController) migrating(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (bool, error) {
	// skip if the phase isn't Migrating and the MigrationResourceDeployed condition is True
	if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating &&
		meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		return false, nil
	}

	// To From: select the registering items to start registering
	var registering []models.ManagedClusterMigration
	db := database.GetGorm()
	err := db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationResourceInitialized,
	}).Find(&registering).Error
	if err != nil {
		return false, err
	}

	// check if the cluster is synced to "To Hub"
	// if not, sent registering event to "From hub", else change the item into registered
	registeringClusters := map[string][]string{}
	registeredClusters := make([]models.ManagedClusterMigration, 0)
	for _, m := range registering {
		cluster := models.ManagedCluster{}
		err := db.Where("payload->'metadata'->>'name' = ?", m.ClusterName).First(&cluster).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) { // error
			return false, err
		} else if errors.Is(err, gorm.ErrRecordNotFound) { // not found -> migrating
			log.Warn("cluster(%s) is not found in the table", m.ClusterName)
		} else { // found
			if cluster.LeafHubName == m.ToHub { // synced into target hub
				log.Info("cluster(%s) is switched to hub(%s) in the table", m.ClusterName, m.ToHub)
				err := db.Where(&models.ManagedClusterMigration{
					ClusterName: m.ClusterName,
					ToHub:       m.ToHub, FromHub: m.FromHub,
				}).Update("stage", migrationv1alpha1.MigrationClusterRegistered).Error
				if err != nil {
					return false, err
				}
				m.Stage = migrationv1alpha1.MigrationClusterRegistered
				registeredClusters = append(registeredClusters, m)
			} else {
				log.Info("cluster(%s) is not switched into hub(%s)", m.ClusterName, m.ToHub)
				registeringClusters[m.FromHub] = append(registeringClusters[m.FromHub], m.ClusterName)
			}
		}
	}

	conditionMessage := "All the migrated clusters have been registered"
	conditionReason := conditionReasonAddonConfigNotDeployed
	conditionStatus := metav1.ConditionTrue
	if len(registeringClusters) > 0 {
		// generate the kubeconfig for these clusters
		bootstrapSecret, err := m.generateBootstrapSecret(ctx, mcm)
		if err != nil {
			return false, err
		}
		// update the condition
		notRegisterClusters := []string{}
		for fromHub, clusters := range registeringClusters {
			notRegisterClusters = append(notRegisterClusters, clusters...)
			err = m.specToFromHub(ctx, fromHub, mcm.Spec.To, migrationv1alpha1.PhaseMigrating, clusters, bootstrapSecret)
			if err != nil {
				return false, err
			}
		}
		// to avoid the status display too many clusters, which are not registered,
		// truncate the cluster list if it has more than 3 clusters
		clusters := notRegisterClusters
		if len(notRegisterClusters) > 3 {
			clusters = append(notRegisterClusters[:3], "...")
		}
		conditionMessage = fmt.Sprintf("The clusters %v are not registered into hub(%s)", clusters,
			mcm.Spec.To)
		conditionReason = conditionReasonClusterNotRegistered
		conditionStatus = metav1.ConditionFalse
		return true, nil
	}

	if len(registeredClusters) > 0 {
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "from_hub"}, {Name: "to_hub"}, {Name: "cluster_name"}},
			UpdateAll: true,
		}).CreateInBatches(registeredClusters, 10).Error
		if err != nil {
			return false, err
		}
	}

	err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationClusterRegistered,
		conditionStatus, conditionReason, conditionMessage)
	if err != nil {
		return false, err
	}

	if len(registeringClusters) > 0 {
		log.Info("waiting clusters to be registered to new hub")
		return true, nil
	}

	// deploy
	var deploying []models.ManagedClusterMigration
	err = db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationClusterRegistered,
	}).Find(&deploying).Error
	if err != nil {
		return false, err
	}

	conditionMessage = "All the migrated cluster configs have been deployed"
	conditionReason = conditionReasonClusterMigrated
	conditionStatus = metav1.ConditionTrue
	if len(deploying) > 0 {
		migratingMessages := []string{}
		for idx, m := range deploying {
			migratingMessages = append(migratingMessages, m.ClusterName)
			// if the migrating clusters more than 3, only show the first 3 items in the condition message
			if idx == 2 {
				migratingMessages = append(migratingMessages, "...")
				break
			}
		}
		conditionMessage = fmt.Sprintf("The cluster configs %v are not deployed into hub(%s)",
			migratingMessages, mcm.Spec.To)
		conditionStatus = metav1.ConditionFalse
		conditionReason = conditionReasonAddonConfigNotDeployed
	}

	err = m.UpdateConditionWithRetry(ctx, mcm, migrationv1alpha1.MigrationResourceDeployed,
		conditionStatus, conditionReason, conditionMessage)
	if err != nil {
		return false, err
	}

	if len(deploying) > 0 {
		return true, nil
	}

	if meta.IsStatusConditionFalse(mcm.Status.Conditions, migrationv1alpha1.MigrationResourceDeployed) {
		log.Info("waiting clusters to be registered to new hub")
		return true, nil
	}
	return false, nil
}

func (m *ClusterMigrationController) generateBootstrapSecret(ctx context.Context,
	mcm *migrationv1alpha1.ManagedClusterMigration,
) (*corev1.Secret, error) {
	toHubCluster := mcm.Spec.To
	// get the secret which is generated by msa
	desiredSecret := &corev1.Secret{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name:      mcm.GetName(),
		Namespace: toHubCluster,
	}, desiredSecret); err != nil {
		return nil, err
	}
	// fetch the managed cluster to get url
	managedCluster := &clusterv1.ManagedCluster{}
	if err := m.Client.Get(ctx, types.NamespacedName{
		Name: toHubCluster,
	}, managedCluster); err != nil {
		return nil, err
	}

	config := clientcmdapi.NewConfig()
	config.Clusters[mcm.GetName()] = &clientcmdapi.Cluster{
		Server:                   managedCluster.Spec.ManagedClusterClientConfigs[0].URL,
		CertificateAuthorityData: desiredSecret.Data["ca.crt"],
	}
	config.AuthInfos["user"] = &clientcmdapi.AuthInfo{
		Token: string(desiredSecret.Data["token"]),
	}
	config.Contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  toHubCluster,
		AuthInfo: "user",
	}
	config.CurrentContext = "default-context"

	// load it into secret
	kubeconfigBytes, err := clientcmd.Write(*config)
	if err != nil {
		return nil, err
	}

	bootstrapSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecretNamePrefix + toHubCluster,
			Namespace: "multicluster-engine",
		},
		Data: map[string][]byte{
			"kubeconfig": kubeconfigBytes,
		},
	}
	return bootstrapSecret, nil
}
