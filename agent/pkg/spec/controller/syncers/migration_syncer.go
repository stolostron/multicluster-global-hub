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
	operatorv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bundleevent "github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
)

type managedClusterMigrationSyncer struct {
	log     logr.Logger
	client  client.Client
	context context.Context
}

func NewManagedClusterMigrationSyncer(context context.Context, client client.Client) *managedClusterMigrationSyncer {
	return &managedClusterMigrationSyncer{
		log:     ctrl.Log.WithName("managed-cluster-migration-syncer"),
		client:  client,
		context: context,
	}
}

func (syncer *managedClusterMigrationSyncer) Sync(payload []byte) error {
	if len(payload) == 0 {
		return syncer.patchClusterManager()
	}

	// handle migration.from cloud event
	managedClusterMigrationEvent := &bundleevent.ManagedClusterMigrationEvent{}
	if err := json.Unmarshal(payload, managedClusterMigrationEvent); err != nil {
		return err
	}

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
	}

	klusterletConfig := managedClusterMigrationEvent.KlusterletConfig
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
		annotations["agent.open-cluster-management.io/klusterlet-config"] = klusterletConfig.Name
		mcl.SetAnnotations(annotations)
		if err := syncer.client.Update(syncer.context, mcl); err != nil {
			return err
		}
	}

	return nil
}

func (syncer *managedClusterMigrationSyncer) patchClusterManager() error {
	clusterManager := &operatorv1.ClusterManager{}
	namespacedName := types.NamespacedName{Name: "cluster-manager"}
	if err := syncer.client.Get(syncer.context, namespacedName, clusterManager); err != nil {
		return err
	}
	patchObject := []byte(`
apiVersion: operator.open-cluster-management.io/v1
kind: ClusterManager
metadata:
  name: cluster-manager
spec:
  registrationConfiguration:
    featureGates:
      - feature: ManagedClusterAutoApproval
        mode: Enable
    autoApproveUsers:
      - system:serviceaccount:open-cluster-management:agent-registration-bootstrap
`)
	// patch cluster-manager to enable ManagedClusterAutoApproval
	if err := syncer.client.Patch(syncer.context, clusterManager, client.RawPatch(types.ApplyPatchType, patchObject), &client.PatchOptions{
		FieldManager: "multicluster-global-hub-agent",
	}); err != nil {
		return err
	}
	return nil
}
