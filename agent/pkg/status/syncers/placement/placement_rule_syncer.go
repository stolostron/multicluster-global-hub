package placement

import (
	"context"

	placementrulesv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func LaunchPlacementRuleSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *configs.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &placementrulesv1.PlacementRule{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// local placementrule
	localShouldUpdate := func(obj client.Object) bool {
		// return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue &&
		// 	!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) // local resource
		return false // disable the placementrule now
	}

	// syncer
	name := "status.placement_rule"
	syncInterval := configmap.GetPolicyDuration

	return generic.LaunchMultiEventSyncer(
		name,
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		syncInterval,
		[]*generic.EmitterHandler{
			{
				Handler: generic.NewGenericHandler(&genericpayload.GenericObjectBundle{},
					generic.WithTweakFunc(cleanupManagedFields),
					generic.WithShouldUpdate(globalResource)),
				Emitter: generic.NewGenericEmitter(enum.PlacementRuleSpecType),
			},

			{
				Handler: generic.NewGenericHandler(&genericpayload.GenericObjectBundle{},
					generic.WithTweakFunc(cleanupManagedFields),
					generic.WithShouldUpdate(localShouldUpdate)),
				Emitter: generic.NewGenericEmitter(enum.LocalPlacementRuleSpecType),
			},
		})
}
