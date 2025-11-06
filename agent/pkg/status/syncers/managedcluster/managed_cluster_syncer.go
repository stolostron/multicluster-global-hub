package managedcluster

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var log = logger.DefaultZapLogger()

var addedManagedClusterSyncer = false

func AddManagedClusterSyncer(ctx context.Context, mgr ctrl.Manager, p transport.Producer,
	periodicSyncer *generic.PeriodicSyncer,
) error {
	if addedManagedClusterSyncer {
		return nil
	}
	// 1. define a emitter for the managed cluster
	clusterEmitter := emitters.NewObjectEmitter(
		enum.ManagedClusterType,
		p,
		// filter out the managed clusters that are migrating and the clusterClaim id not ready,
		// don't filter out the managed hub clusters, it might need to be removed from the cluster inventory in the db
		emitters.WithPredicateFunc(predicate.NewPredicateFuncs(targetFunc)),
		emitters.WithTargetFunc(targetFunc),
		// the cluster role might be change between managed cluster and managed hub
		emitters.WithShouldDeleteFunc(func(object client.Object) bool {
			return utils.HasLabel(object, constants.GHDeployModeLabelKey)
		}),
		emitters.WithTweakFunc(clusterTweakFunc),       // clean unnecessary fields, like managedFields
		emitters.WithMetadataFunc(clusterMetadataFunc), // extract metadata from object, use clusterClaimId as the object id
	)

	// 2. add the emitter to controller
	if err := generic.AddSyncCtrl(
		mgr,
		"managedcluster",
		func() client.Object { return &clusterv1.ManagedCluster{} },
		clusterEmitter,
	); err != nil {
		return err
	}

	// 3. register the emitter to periodic syncer
	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) {
			var clusters clusterv1.ManagedClusterList
			if err := mgr.GetClient().List(ctx, &clusters); err != nil {
				return nil, err
			}
			// filter out the managed clusters that are migrating
			var filtered []client.Object
			for i := range clusters.Items {
				obj := &clusters.Items[i]
				if targetFunc(obj) && !utils.HasLabel(obj, constants.GHDeployModeLabelKey) {
					log.Infof("valid cluster: %s", obj.GetName())
					filtered = append(filtered, obj)
				}
			}
			return filtered, nil
		},
		Emitter: clusterEmitter,
	})

	addedManagedClusterSyncer = true
	return nil
}

func targetFunc(object client.Object) bool {
	return !utils.HasAnnotation(object, constants.ManagedClusterMigrating) &&
		getClusterClaimID(object) != ""
}

func clusterTweakFunc(object client.Object) {
	utils.MergeAnnotations(object, map[string]string{
		constants.ManagedClusterManagedByAnnotation: configs.GetLeafHubName(),
	})
	object.SetManagedFields(nil)
}

func clusterMetadataFunc(object client.Object) *genericbundle.ObjectMetadata {
	return &genericbundle.ObjectMetadata{
		ID:        getClusterClaimID(object),
		Namespace: object.GetNamespace(),
		Name:      object.GetName(),
	}
}

func getClusterClaimID(object client.Object) string {
	cluster, ok := object.(*clusterv1.ManagedCluster)
	if !ok {
		log.Errorf("wrong instance passed to tweak function, not a ManagedCluster: %v", object)
		return ""
	}
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			return claim.Value
		}
	}
	return ""
}
