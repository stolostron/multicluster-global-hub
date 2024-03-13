package placement

import (
	"context"

	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
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

func LaunchPlacementRuleSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &placementrulesv1.PlacementRule{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })
	controller := generic.NewGenericController(instance, predicate)

	// emitter config
	tweakFunc := func(obj client.Object) {
		obj.SetManagedFields(nil)
	}
	globalPlacementRuleEmitter := generic.ObjectEmitterWrapper(enum.PlacementRuleSpecType,
		func(obj client.Object) bool {
			return agentConfig.EnableGlobalResource &&
				utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
		}, tweakFunc)

	localPlacementRuleEmitter := generic.ObjectEmitterWrapper(enum.LocalPlacementRuleSpecType,
		func(obj client.Object) bool {
			return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue &&
				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) // local resource
		}, tweakFunc)

	// syncer
	name := "status.placement_decision"
	syncInterval := statusconfig.GetPolicyDuration

	return generic.LaunchGenericObjectSyncer(
		name,
		mgr,
		controller,
		producer,
		syncInterval,
		[]generic.ObjectEmitter{
			globalPlacementRuleEmitter,
			localPlacementRuleEmitter,
		})
}
