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
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
)

func AddManagedClusterSyncer(ctx context.Context, mgr ctrl.Manager, p transport.Producer,
	periodicSyncer *generic.PeriodicSyncer,
) error {
	// 1. define a emitter for the managed cluster
	clusterEmitter := emitters.NewEmitter(
		enum.ManagedClusterType,
		p,
		emitters.WithEventFilterFunc(predicate.NewPredicateFuncs(validCluster)),
		emitters.WithTweakFunc(clusterTweakFunc),
	)

	// 2. add the emitter to controller
	if err := generic.AddSyncCtrl(
		mgr,
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
				if validCluster(obj) {
					filtered = append(filtered, obj)
				}
			}
			return filtered, nil
		},
		Emitter: clusterEmitter,
	})
	return nil
}

// validCluster will filter out:
// 1. the managed clusters that are migrating
// 2. the clusterClaim id not ready
// 3. the managed cluster is managedhub
func validCluster(object client.Object) bool {
	return !utils.HasAnnotation(object, constants.ManagedClusterMigrating) &&
		!utils.HasLabel(object, constants.GHDeployModeLabelKey) &&
		getClusterClaimID(object) != ""
}

func clusterTweakFunc(object client.Object) {
	utils.MergeAnnotations(object, map[string]string{
		constants.ManagedClusterManagedByAnnotation: configs.GetLeafHubName(),
	})
	object.SetUID(types.UID(getClusterClaimID(object)))
}

func getClusterClaimID(object client.Object) string {
	cluster, ok := object.(*clusterv1.ManagedCluster)
	if !ok {
		panic("Wrong instance passed to tweak function, not a ManagedCluster")
	}
	for _, claim := range cluster.Status.ClusterClaims {
		if claim.Name == "id.k8s.io" {
			return claim.Value
		}
	}
	return ""
}
