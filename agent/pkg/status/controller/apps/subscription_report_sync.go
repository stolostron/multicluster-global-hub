package apps

import (
	"context"
	"fmt"

	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddSubscriptionReportsSyncer adds subscription-report controller to the manager.
func AddSubscriptionReportsSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionReport{} }
	leafHubName := statusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.SubscriptionReportMsgKey),
			genericbundle.NewGenericStatusBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send subscription report.

	if err := generic.NewGenericStatusSyncer(mgr, "subscriptions-reports-sync", producer,
		bundleCollection, createObjFunction, nil, statusconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add subscription reports controller to the manager - %w", err)
	}

	return nil
}

func LaunchSubscriptionReportSyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer) error {

	// controller config
	instance := func() client.Object { return &appsv1alpha1.SubscriptionReport{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// emitter config
	emitter := generic.ObjectEmitterWrapper(enum.SubscriptionReportType, nil, nil)

	// syncer
	name := "status.subscription_report"
	syncInterval := statusconfig.GetPolicyDuration

	return generic.LaunchGenericObjectSyncer(
		name,
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		syncInterval,
		[]generic.ObjectEmitter{
			emitter,
		})
}
