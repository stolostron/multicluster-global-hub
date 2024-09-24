package syncers

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	klusterletv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
)

type managedClusterMigrationSyncer struct {
	log    logr.Logger
	client client.Client
}

func NewManagedClusterMigrationSyncer(client client.Client) *managedClusterMigrationSyncer {
	return &managedClusterMigrationSyncer{
		log:    ctrl.Log.WithName("managed-cluster-migration-syncer"),
		client: client,
	}
}

func (syncer *managedClusterMigrationSyncer) Sync(payload []byte) error {
	if len(payload) == 0 {
		// TODO: handle migration.to cloud event
		return nil
	}

	// handle migration.from cloud event
	managedClusterMigrationEvent := &bundleevent.ManagedClusterMigrationEvent{}
	if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
		return err
	}

	bootstrapSecret := managedClusterMigrationEvent.BootstrapSecret

	foundBootstrapSecret := &corev1.Secret{}
	if err := syncer.client.Get(context.TODO(),
		types.NamespacedName{
			Name:      bootstrapSecret.Name,
			Namespace: bootstrapSecret.Namespace,
		}, foundBootstrapSecret); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating bootstrap secret", "bootstrap secret", bootstrapSecret)
			if err := syncer.client.Create(context.TODO(), bootstrapSecret); err != nil {
				return err
			}
		} else {
			return err
		}
	}

	klusterletConfig := managedClusterMigrationEvent.KlusterletConfig
	foundKlusterletConfig := &klusterletv1alpha1.KlusterletConfig{}
	if err := syncer.client.Get(context.TODO(),
		types.NamespacedName{
			Name: klusterletConfig.Name,
		}, foundKlusterletConfig); err != nil {
		if apierrors.IsNotFound(err) {
			syncer.log.Info("creating klusterlet config", "klusterlet config", klusterletConfig)
			if err := syncer.client.Create(context.TODO(), klusterletConfig); err != nil {
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
		if err := syncer.client.Get(context.TODO(), types.NamespacedName{
			Name: managedCluster,
		}, mcl); err != nil {
			return err
		}
		annotations := mcl.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["agent.open-cluster-management.io/klusterlet-config"] = klusterletConfig.Name
		mcl.SetAnnotations(annotations)
		if err := syncer.client.Update(context.TODO(), mcl); err != nil {
			return err
		}
	}

	return nil
}
