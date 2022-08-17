package localpolicies

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/syncintervals"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/transport/producer"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

const (
	localPoliciesStatusSyncLog = "local-policies-status-sync"
	rootPolicyLabel            = "policy.open-cluster-management.io/root-policy"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalPoliciesController(mgr ctrl.Manager, transport producer.Producer, leafHubName string,
	incarnation uint64, hubOfHubsConfig *corev1.ConfigMap, syncIntervalsData *syncintervals.SyncIntervals,
) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	bundleCollection := createBundleCollection(leafHubName, incarnation, hubOfHubsConfig)

	localPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !helper.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			!helper.HasLabel(object, rootPolicyLabel)
	})

	if err := generic.NewGenericStatusSyncController(mgr, localPoliciesStatusSyncLog, transport, bundleCollection,
		createObjFunc, localPolicyPredicate, syncIntervalsData.GetPolicies); err != nil {
		return fmt.Errorf("failed to add local policies controller to the manager - %w", err)
	}

	return nil
}

func createBundleCollection(leafHubName string, incarnation uint64,
	hubOfHubsConfig *corev1.ConfigMap,
) []*generic.BundleCollectionEntry {
	extractLocalPolicyIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }

	// clusters per policy (base bundle)
	localClustersPerPolicyTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalClustersPerPolicyMsgKey)
	localClustersPerPolicyBundle := grc.NewClustersPerPolicyBundle(leafHubName, incarnation,
		extractLocalPolicyIDFunc)

	// compliance status bundle
	localCompleteComplianceStatusTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalPolicyCompleteComplianceMsgKey)
	localCompleteComplianceStatusBundle := grc.NewCompleteComplianceStatusBundle(leafHubName,
		localClustersPerPolicyBundle, incarnation, extractLocalPolicyIDFunc)

	localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPolicySpecMsgKey)
	localPolicySpecBundle := bundle.NewGenericStatusBundle(leafHubName, incarnation, cleanPolicy)

	// check for full information
	localPolicyStatusPredicate := func() bool {
		return hubOfHubsConfig.Data["aggregationLevel"] == "full" &&
			hubOfHubsConfig.Data["enableLocalPolicies"] == "true"
	}
	// multiple bundles for local policies
	return []*generic.BundleCollectionEntry{
		generic.NewBundleCollectionEntry(localClustersPerPolicyTransportKey,
			localClustersPerPolicyBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localCompleteComplianceStatusTransportKey,
			localCompleteComplianceStatusBundle, localPolicyStatusPredicate),
		generic.NewBundleCollectionEntry(localPolicySpecTransportKey, localPolicySpecBundle,
			func() bool { return hubOfHubsConfig.Data["enableLocalPolicies"] == "true" }),
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
