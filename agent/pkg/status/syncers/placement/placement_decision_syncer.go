package placement

import (
	"context"

	clustersv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
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

func LaunchPlacementDecisionSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *configs.AgentConfig,
	producer transport.Producer,
) error {
	return generic.LaunchMultiEventSyncer(
		"status.placement_decision",
		mgr,
		generic.NewGenericController(
			func() client.Object { return &clustersv1beta1.PlacementDecision{} },         // instance
			predicate.NewPredicateFuncs(func(object client.Object) bool { return true }), // predicate
		),
		producer,
		configmap.GetPolicyDuration,
		[]*generic.EmitterHandler{
			{
				Handler: generic.NewGenericHandler(
					&genericpayload.GenericObjectBundle{},
					generic.WithTweakFunc(cleanupManagedFields),
					generic.WithShouldUpdate(globalResource),
				),
				Emitter: generic.NewGenericEmitter(enum.PlacementDecisionType),
			},
		})
}

func cleanupManagedFields(obj client.Object) {
	obj.SetManagedFields(nil)
}

func globalResource(obj client.Object) bool {
	return utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation)
}
