package syncers

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
)

const (
	bootstrapSecretBackupSuffix = "-backup"
)

type managedClusterMigrationFromSyncer struct {
	log     logr.Logger
	client  client.Client
	context context.Context
}

func NewManagedClusterMigrationFromSyncer(context context.Context, client client.Client) *managedClusterMigrationFromSyncer {
	return &managedClusterMigrationFromSyncer{
		log:     ctrl.Log.WithName("managed-cluster-migration-from-syncer"),
		client:  client,
		context: context,
	}
}

func (syncer *managedClusterMigrationFromSyncer) Sync(payload []byte) error {
	// handle migration.from cloud event
	managedClusterMigrationEvent := &bundleevent.ManagedClusterMigrationFromEvent{}
	if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
		return err
	}

	// create or update bootstrap secret
	bootstrapSecret := managedClusterMigrationEvent.BootstrapSecret
	foundBootstrapSecret := &corev1.Secret{}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating bootstrap secret", "bootstrap secret", bootstrapSecret)
			if err := syncer.client.Create(syncer.context, bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap secret if it already exists
		syncer.log.Info("updating bootstrap secret", "bootstrap secret", bootstrapSecret)
		if err := syncer.client.Update(syncer.context, bootstrapSecret); err != nil {
			return err
		}
	}

	// create or update boostrap secret backup
	bootstrapSecretBackup := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapSecret.Name + bootstrapSecretBackupSuffix,
			Namespace: bootstrapSecret.Namespace,
		},
		Data: bootstrapSecret.Data,
	}
	foundBootstrapSecretBackup := &corev1.Secret{}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name:      bootstrapSecretBackup.Name,
			Namespace: bootstrapSecretBackup.Namespace,
		}, foundBootstrapSecretBackup); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating bootstrap backup secret", "bootstrap backup secret", bootstrapSecretBackup)
			if err := syncer.client.Create(syncer.context, bootstrapSecretBackup); err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// update the bootstrap backup secret if it already exists
		syncer.log.Info("updating bootstrap backup secret", "bootstrap backup secret", bootstrapSecretBackup)
		if err := syncer.client.Update(syncer.context, bootstrapSecret); err != nil {
			return err
		}
	}

	// create klusterlet config if it does not exist
	klusterletConfig := managedClusterMigrationEvent.KlusterletConfig
	// set the bootstrap kubeconfig secrets in klusterlet config
	klusterletConfig.Spec.BootstrapKubeConfigs.LocalSecrets.KubeConfigSecrets = []operatorv1.KubeConfigSecret{
		{
			Name: bootstrapSecret.Name,
		},
		{
			Name: bootstrapSecretBackup.Name,
		},
	}
	foundKlusterletConfig := &klusterletv1alpha1.KlusterletConfig{}
	if err := syncer.client.Get(syncer.context,
		types.NamespacedName{
			Name: klusterletConfig.Name,
		}, foundKlusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating klusterlet config", "klusterlet config", klusterletConfig)
			if err := syncer.client.Create(syncer.context, klusterletConfig); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// update managed cluster annotations to point to the new klusterlet config
	managedClusters := managedClusterMigrationEvent.ManagedClusters
	for _, managedCluster := range managedClusters {
		mcl := &clusterv1.ManagedCluster{}
		if err := syncer.client.Get(syncer.context, types.NamespacedName{
			Name: managedCluster,
		}, mcl); err != nil {
			return err
		}
		annotations := mcl.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}
		if annotations["agent.open-cluster-management.io/klusterlet-config"] == klusterletConfig.Name {
			continue
		}
		annotations["agent.open-cluster-management.io/klusterlet-config"] = klusterletConfig.Name
		mcl.SetAnnotations(annotations)
		if err := syncer.client.Update(syncer.context, mcl); err != nil {
			return err
		}
	}

	return nil
}
