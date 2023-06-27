package localpolicies

import (
	"fmt"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalClusterPoliciesController(mgr ctrl.Manager, producer transport.Producer, leafHubName string,
	incarnation uint64, hubOfHubsConfig *corev1.ConfigMap, syncIntervalsData *config.SyncIntervals,
) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }

	localClusterPolicyHistoryEventTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalClusterPolicyHistoryEventsMsgKey)
	clusterPolicyHistoryEventBundle := grc.NewClusterPolicyHistoryEventBundle(leafHubName, incarnation, mgr.GetClient())

	localClusterPolicyBundleEntryCollection := []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localClusterPolicyHistoryEventTransportKey, clusterPolicyHistoryEventBundle,
			func() bool { return hubOfHubsConfig.Data["enableLocalPolicies"] == "true" }),
	}

	localClusterPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			helper.HasLabel(object, rootPolicyLabel)
	})
	if err := generic.NewGenericStatusSyncController(mgr, localPoliciesStatusSyncLog, producer,
		localClusterPolicyBundleEntryCollection, createObjFunc, localClusterPolicyPredicate,
		syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local cluster policies controller to the manager - %w", err)
	}

	return nil
}

func createClusterPolicyBundleCollection(leafHubName string, incarnation uint64,
	hubOfHubsConfig *corev1.ConfigMap, runtimeClient client.Client,
) []*generic.BundleCollectionEntry {
	// clusters per policy (base bundle)
	localClusterPolicyHistoryEventTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalClusterPolicyHistoryEventsMsgKey)
	clusterPolicyHistoryEventBundle := grc.NewClusterPolicyHistoryEventBundle(leafHubName, incarnation, runtimeClient)
	// multiple bundles for local policies
	return []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localClusterPolicyHistoryEventTransportKey, clusterPolicyHistoryEventBundle,
			func() bool { return hubOfHubsConfig.Data["enableLocalPolicies"] == "true" }),
	}
}
