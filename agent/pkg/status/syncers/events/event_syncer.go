package events

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/events/handlers"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var addedEventSyncer = false

func LaunchEventSyncer(ctx context.Context, mgr ctrl.Manager,
	agentConfig *configs.AgentConfig, producer transport.Producer,
) error {
	if addedEventSyncer {
		return nil
	}
	// controller
	instance := func() client.Object { return &corev1.Event{} }
	eventPredicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event || extend other InvolvedObject kind
		return event.InvolvedObject.Kind == policiesv1.Kind ||
			event.InvolvedObject.Kind == constants.ManagedClusterKind ||
			event.InvolvedObject.Kind == constants.ClusterGroupUpgradeKind
	})

	err := generic.LaunchMultiEventSyncer(
		"status.event",
		mgr,
		generic.NewGenericController(instance, eventPredicate),
		producer,
		configmap.GetEventDuration,
		[]*generic.EmitterHandler{
			{
				Handler: handlers.NewLocalRootPolicyEventHandler(ctx, mgr.GetClient()),
				Emitter: handlers.NewRootPolicyEventEmitter(enum.LocalRootPolicyEventType),
			},
			{
				Handler: handlers.NewManagedClusterEventHandler(ctx, mgr.GetClient()),
				Emitter: handlers.NewManagedClusterEventEmitter(),
			},
			{
				Handler: handlers.NewClusterGroupUpgradeEventHandler(ctx, mgr.GetClient()),
				Emitter: handlers.NewClusterGroupUpgradeEventEmitter(),
			},
		})
	if err != nil {
		return err
	}
	addedEventSyncer = true
	return nil
}
