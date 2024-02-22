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
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func LaunchManagedClusterSyncer(ctx context.Context, mgr ctrl.Manager,
	agentConfig *config.AgentConfig, producer transport.Producer) error {

	statusConfig := agentConfig.TransportConfig.KafkaConfig.Topics.StatusTopic
	return generic.LaunchGenericObjectSyncer(
		"status.managed_cluster",
		mgr,
		generic.NewGenericController(clusterInstance, clusterPredicate()),
		producer,
		statusconfig.GetPolicyDuration,
		[]generic.ObjectEmitter{
			NewManagedClusterEmitter(statusConfig),
		})
}

func clusterInstance() client.Object {
	return &clusterv1.ManagedCluster{}
}

func clusterPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return true
	})
}
