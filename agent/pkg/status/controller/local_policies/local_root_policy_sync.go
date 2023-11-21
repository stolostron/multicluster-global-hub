package localpolicies

import (
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	rootPolicyLabel = "policy.open-cluster-management.io/root-policy"
)

// AddLocalPoliciesSyncer this function adds a new local policies sync controller.
func AddLocalPoliciesSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	bundleCollection := createBundleCollection(mgr)

	localPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			!helper.HasLabel(object, rootPolicyLabel)
	})

	return generic.NewGenericStatusSyncController(mgr, "local-policies-status-sync", producer, bundleCollection,
		createObjFunc, localPolicyPredicate, config.GetPolicyDuration)
}

func createBundleCollection(mgr ctrl.Manager) []*generic.BundleCollectionEntry {
	leafHubName := config.GetLeafHubName()

	extractLocalPolicyIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := grc.NewClustersPerPolicyBundle(leafHubName, extractLocalPolicyIDFunc)

	// compliance status bundle
	localCompleteComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalPolicyCompleteComplianceMsgKey)
	localCompleteComplianceStatusBundle := grc.NewCompleteComplianceStatusBundle(leafHubName,
		localClustersPerPolicyBundle, extractLocalPolicyIDFunc)

	// local spec policy bundle
	localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPolicySpecMsgKey)
	localPolicySpecBundle := bundle.NewGenericStatusBundle(leafHubName, cleanPolicy)

	// policy history event bundle
	localClusterPolicyHistoryEventTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalClusterPolicyStatusEventMsgKey)
	clusterPolicyHistoryEventBundle := grc.NewClusterPolicyHistoryEventBundle(leafHubName, mgr.GetClient())

	// check for full information
	localPolicyStatusPredicate := func() bool {
		return config.GetAggregationLevel() == config.AggregationFull &&
			config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue
	}

	// multiple bundles for local policies
	return []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localCompleteComplianceStatusTransportKey,
			localCompleteComplianceStatusBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localPolicySpecTransportKey, localPolicySpecBundle,
			func() bool { return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue }),
		generic.NewBundleCollectionEntry(localClusterPolicyHistoryEventTransportKey, clusterPolicyHistoryEventBundle,
			func() bool { return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue }),
	}
}

// status will be sent in the policy status bundles.
func cleanPolicy(object bundle.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean policy function, not a Policy")
	}
	policy.Status = policiesv1.PolicyStatus{}
}
