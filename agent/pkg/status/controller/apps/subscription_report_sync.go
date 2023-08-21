package apps

import (
	"fmt"

	appsv1alpha1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	agentstatusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	subscriptionReportsSyncLog = "subscriptions-reports-sync"
)

// AddSubscriptionReportsController adds subscription-report controller to the manager.
func AddSubscriptionReportsController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionReport{} }
	leafHubName := agentstatusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, constants.SubscriptionReportMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send subscription report.

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionReportsSyncLog, producer, bundleCollection,
		createObjFunction, nil, agentstatusconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add subscription reports controller to the manager - %w", err)
	}

	return nil
}
