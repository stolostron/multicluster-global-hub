package apps

import (
	"context"

	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
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

func LaunchSubscriptionReportSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *configs.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &appsv1alpha1.SubscriptionReport{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter, handler
	eventData := genericpayload.GenericObjectBundle{}

	return generic.LaunchMultiEventSyncer(
		"status.subscription_report",
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		configmap.GetPolicyDuration,
		[]*generic.EmitterHandler{
			{
				Handler: generic.NewGenericHandler(&eventData),
				Emitter: generic.NewGenericEmitter(enum.SubscriptionReportType),
			},
		})
}
