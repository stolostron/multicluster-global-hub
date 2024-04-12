package placement

import (
	"context"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
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

func LaunchPlacementDecisionSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &clustersv1beta1.PlacementDecision{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter config
	placementDecisionEmitter := generic.ObjectEmitterWrapper(enum.PlacementDecisionType,
		func(obj client.Object) bool {
			return utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) // global resource
		}, func(obj client.Object) {
			obj.SetManagedFields(nil)
		}, false)

	// syncer
	name := "status.placement_decision"
	syncInterval := statusconfig.GetPolicyDuration

	return generic.LaunchGenericObjectSyncer(
		name,
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		syncInterval,
		[]generic.ObjectEmitter{
			placementDecisionEmitter,
		})
}
