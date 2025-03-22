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
	if mcm.Status.Phase != migrationv1alpha1.PhaseMigrating &&
		meta.IsStatusConditionTrue(mcm.Status.Conditions, migrationv1alpha1.MigrationClusterRegistered) {
		return false, nil
	}

	log.Info("migration registering")

	condType := migrationv1alpha1.MigrationClusterRegistered
	condStatus := metav1.ConditionTrue
	condReason := conditionReasonClusterRegistered
	condMsg := "All the migrated clusters have been registered"
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

	// To From: select the initialized items to start initialized
	var initialized []models.ManagedClusterMigration
	db := database.GetGorm()
	err = db.Where(&models.ManagedClusterMigration{
		Stage: migrationv1alpha1.MigrationResourceInitialized,
	}).Find(&initialized).Error
	if err != nil {
		return false, err
	}

	// check if the cluster is synced to "To Hub"
	// if not, sent registering event to "From hub", else change the item into registered
	registeringClusters := map[string][]string{}
	updateRegisteredClusters := make([]models.ManagedClusterMigration, 0)
	registeredClusters := map[string][]string{}
	for _, m := range initialized {
		cluster := models.ManagedCluster{}
		err := db.Where("payload->'metadata'->>'name' = ?", m.ClusterName).First(&cluster).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) { // error
			return false, err
		} else if errors.Is(err, gorm.ErrRecordNotFound) { // not found -> migrating
			condStatus = metav1.ConditionFalse
			condReason = conditionReasonClusterNotRegistered
			condMsg = fmt.Sprintf("cluster(%s) is not found in the table", m.ClusterName)
			log.Warn(condMsg)
			registeringClusters[m.FromHub] = append(registeringClusters[m.FromHub], m.ClusterName)
		} else { // found
			if cluster.LeafHubName == m.ToHub { // synced into target hub
				log.Infof("cluster(%s) is switched to hub(%s) in the table", m.ClusterName, m.ToHub)
				err := db.Model(&models.ManagedClusterMigration{}).Where(&models.ManagedClusterMigration{
					ClusterName: m.ClusterName,
					ToHub:       m.ToHub, FromHub: m.FromHub,
				}).Update("stage", migrationv1alpha1.MigrationClusterRegistered).Error
				if err != nil {
					return false, err
				}
				m.Stage = migrationv1alpha1.MigrationClusterRegistered
				updateRegisteredClusters = append(updateRegisteredClusters, m)
				registeredClusters[m.FromHub] = append(registeredClusters[m.FromHub], m.ClusterName)
			} else {
				log.Infof("cluster(%s) is not switched into hub(%s)", m.ClusterName, m.ToHub)
				registeringClusters[m.FromHub] = append(registeringClusters[m.FromHub], m.ClusterName)
			}
		}
	}

	// forward the bootstrap secret to the source hub
	bootstrapSecret, err := m.generateBootstrapSecret(ctx, mcm)
	if err != nil {
		return false, err
	}

	if len(updateRegisteredClusters) > 0 {
		// registed successful, send complated event: detach the clusters, config and secret
		for sourceHub, clusters := range registeredClusters {
			err = m.sendEventToSourceHub(ctx, sourceHub, mcm.Spec.To, migrationv1alpha1.PhaseCompleted,
				clusters, bootstrapSecret)
		}

		// update the registered migration items in database
		err := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "from_hub"}, {Name: "to_hub"}, {Name: "cluster_name"}},
			UpdateAll: true,
		}).CreateInBatches(updateRegisteredClusters, 10).Error
		if err != nil {
			return false, err
		}
	}

	if len(registeringClusters) > 0 {
		// sending to from hub cluster with migrating notifications
		notRegisterClusters := []string{}
		for fromHub, clusters := range registeringClusters {
			notRegisterClusters = append(notRegisterClusters, clusters...)
			err = m.sendEventToSourceHub(ctx, fromHub, mcm.Spec.To, migrationv1alpha1.PhaseMigrating, clusters,
				bootstrapSecret)
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
		condMsg = fmt.Sprintf("The clusters %v are not registered into hub(%s)", clusters,
			mcm.Spec.To)
		condReason = conditionReasonClusterNotRegistered
		condStatus = metav1.ConditionFalse

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
	config.Clusters[toHubCluster] = &clientcmdapi.Cluster{
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
