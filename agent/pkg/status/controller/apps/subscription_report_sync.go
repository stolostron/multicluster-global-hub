package apps

import (
	"fmt"

	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	agentconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	statusgeneric "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// AddSubscriptionReportsSyncer adds subscription-report controller to the manager.
func AddSubscriptionReportsSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionReport{} }
	leafHubName := agentconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(fmt.Sprintf("%s.%s", leafHubName, constants.SubscriptionReportMsgKey),
			statusgeneric.NewStatusGenericBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send subscription report.

	if err := generic.NewStatusGenericSyncer(mgr, "subscriptions-reports-sync", producer,
		bundleCollection, createObjFunction, nil, agentconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add subscription reports controller to the manager - %w", err)
	}

	return nil
}
