package managedcluster

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func LaunchManagedClusterSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *configs.AgentConfig,
	producer transport.Producer,
) error {
	// emitter handler config
	eventData := genericpayload.GenericObjectBundle{}
	tweakFunc := func(object client.Object) {
		utils.MergeAnnotations(object, map[string]string{
			constants.ManagedClusterManagedByAnnotation: configs.GetLeafHubName(),
		})
	}
	shouldUpdate := func(obj client.Object) bool { return !utils.HasAnnotation(obj, constants.ManagedClusterMigrating) }

	return generic.LaunchMultiEventSyncer(
		"status.managed_cluster",
		mgr,
		generic.NewGenericController(
			func() client.Object { return &clusterv1.ManagedCluster{} },
			predicate.NewPredicateFuncs(func(object client.Object) bool {
				// If the managed cluster is managedhub do not sync it
				return !utils.HasLabel(object, constants.GHDeployModeLabelKey)
			})),
		producer,
		configmap.GetManagerClusterDuration,
		[]*generic.EmitterHandler{
			{
				Handler: generic.NewGenericHandler(&eventData, generic.WithTweakFunc(tweakFunc),
					generic.WithShouldUpdate(shouldUpdate)),
				Emitter: generic.NewGenericEmitter(enum.ManagedClusterType),
			},
		})
}
