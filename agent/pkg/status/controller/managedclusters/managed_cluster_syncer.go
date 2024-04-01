package managedclusters

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func LaunchManagedClusterSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &clusterv1.ManagedCluster{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter config
	tweakFunc := func(object client.Object) {
		utils.MergeAnnotations(object, map[string]string{
			constants.ManagedClusterManagedByAnnotation: statusconfig.GetLeafHubName(),
		})
	}
	emitter := generic.ObjectEmitterWrapper(enum.ManagedClusterType, nil, tweakFunc, false)

	return generic.LaunchGenericObjectSyncer(
		"status.managed_cluster",
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		statusconfig.GetManagerClusterDuration,
		[]generic.ObjectEmitter{
			emitter,
		})
}
