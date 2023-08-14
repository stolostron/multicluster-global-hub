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
	subscriptionStatusSyncLog = "subscriptions-statuses-sync"
)

// AddSubscriptionStatusesController adds subscription-status controller to the manager.
func AddSubscriptionStatusesController(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunction := func() bundle.Object { return &appsv1alpha1.SubscriptionStatus{} }
	leafHubName := agentstatusconfig.GetLeafHubName()

	bundleCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(fmt.Sprintf("%s.%s", leafHubName, constants.SubscriptionStatusMsgKey),
			bundle.NewGenericStatusBundle(leafHubName, nil),
			func() bool { return true }),
	} // bundle predicate - always send subscription status.

	if err := generic.NewGenericStatusSyncController(mgr, subscriptionStatusSyncLog, producer, bundleCollection,
		createObjFunction, nil, agentstatusconfig.GetPolicyDuration); err != nil {
		return fmt.Errorf("failed to add subscription statuses controller to the manager - %w", err)
	}

	return nil
}
