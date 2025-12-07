package events

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var (
	log            = logger.DefaultZapLogger()
	addEventSyncer = false
	runtimeClient  client.Client
)

func AddEventSyncer(ctx context.Context, mgr ctrl.Manager,
	producer transport.Producer, periodicSyncer *generic.PeriodicSyncer,
) error {
	if addEventSyncer {
		return nil
	}

	runtimeClient = mgr.GetClient()

	managedClusterEventEmitter := emitters.NewEventEmitter(
		enum.ManagedClusterEventType,
		producer,
		runtimeClient,
		managedClusterEventPredicate,
		managedClusterEventTransform,
		emitters.WithPostSend(managedClusterPostSend),
	)

	localRootPolicyEventEmitter := emitters.NewEventEmitter(
		enum.LocalRootPolicyEventType,
		producer,
		runtimeClient,
		localRootPolicyEventPredicate,
		localRootPolicyEventTransform,
		emitters.WithPostSend(localRootPolicyPostSend),
	)

	clusterGroupUpgradeEventEmitter := emitters.NewEventEmitter(
		enum.ClusterGroupUpgradesEventType,
		producer,
		runtimeClient,
		clusterGroupUpgradeEventPredicate,
		clusterGroupUpgradeEventTransform,
		emitters.WithPostSend(clusterGroupUpgradePostSend),
	)

	provisionEventEmitter := emitters.NewEventEmitter(
		enum.ProvisionEventType,
		producer,
		runtimeClient,
		provisionEventPredicate,
		provisionEventTransform,
		emitters.WithPostSend(provisionPostSend),
	)

	// 2. add the emitter to controller
	if err := generic.AddSyncCtrl(
		mgr,
		"event",
		func() client.Object { return &corev1.Event{} },
		managedClusterEventEmitter, localRootPolicyEventEmitter, clusterGroupUpgradeEventEmitter, provisionEventEmitter,
	); err != nil {
		return err
	}

	// 3. register the emitter to periodic syncer
	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) { return listEvents(ctx, runtimeClient) },
		Emitter:  managedClusterEventEmitter,
	})

	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) { return listEvents(ctx, runtimeClient) },
		Emitter:  localRootPolicyEventEmitter,
	})

	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) { return listEvents(ctx, runtimeClient) },
		Emitter:  clusterGroupUpgradeEventEmitter,
	})

	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) { return listEvents(ctx, runtimeClient) },
		Emitter:  provisionEventEmitter,
	})

	addEventSyncer = true
	return nil
}

func listEvents(ctx context.Context, runtimeClient client.Client) ([]client.Object, error) {
	var events corev1.EventList
	if err := runtimeClient.List(ctx, &events); err != nil {
		return nil, err
	}
	items := make([]client.Object, 0, len(events.Items))
	for _, event := range events.Items {
		items = append(items, &event)
	}
	return items, nil
}
