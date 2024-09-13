package managedclusters

import (
	"context"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
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

// LaunchManagedClusterInfoSyncer only for the restful client
func LaunchManagedClusterInfoSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer,
) error {
	if agentConfig.TransportConfig.TransportType != string(transport.Rest) {
		return nil
	}

	// controller config
	instance := func() client.Object { return &clusterinfov1beta1.ManagedClusterInfo{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter config
	tweakFunc := func(object client.Object) {
		utils.MergeAnnotations(object, map[string]string{
			constants.ManagedClusterManagedByAnnotation: statusconfig.GetLeafHubName(),
		})
	}
	emitter := generic.ObjectEmitterWrapper(enum.ManagedClusterInfoType, func(obj client.Object) bool {
		return true
	}, tweakFunc, false)

	return generic.LaunchGenericObjectSyncer(
		"status.managed_cluster_info",
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		statusconfig.GetManagerClusterDuration,
		[]generic.ObjectEmitter{
			emitter,
		})
}
