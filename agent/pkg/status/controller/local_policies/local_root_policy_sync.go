package localpolicies

import (
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	genericbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	rootPolicyLabel = "policy.open-cluster-management.io/root-policy"
)

// AddLocalRootPolicySyncer this function adds a new local policies sync controller.
func AddLocalRootPolicySyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	bundleCollection := createBundleCollection(mgr)

	localPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			!utils.HasLabel(object, rootPolicyLabel)
	})

	return generic.NewGenericStatusSyncer(mgr, "local-root-policies-status-sync", producer, bundleCollection,
		createObjFunc, localPolicyPredicate, config.GetPolicyDuration)
}

func createBundleCollection(mgr ctrl.Manager) []*generic.BundleEntry {
	leafHubName := config.GetLeafHubName()

	extractLocalPolicyIDFunc := func(obj bundle.Object) (string, bool) { return string(obj.GetUID()), true }

	// compliance bundle (base bundle)
	localComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalComplianceMsgKey)
	localComplianceBundle := grc.NewAgentComplianceBundle(leafHubName, extractLocalPolicyIDFunc)

	// complete compliance bundle
	localCompleteComplianceTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalCompleteComplianceMsgKey)
	localCompleteComplianceBundle := grc.NewAgentCompleteComplianceBundle(leafHubName,
		localComplianceBundle, extractLocalPolicyIDFunc)

	// local spec policy bundle
	localPolicySpecTransportKey := fmt.Sprintf("%s.%s", leafHubName, constants.LocalPolicySpecMsgKey)
	localPolicySpecBundle := genericbundle.NewGenericStatusBundle(leafHubName, cleanPolicy)

	// check for full information
	localPolicyStatusPredicate := func() bool {
		return config.GetAggregationLevel() == config.AggregationFull &&
			config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue
	}
	// multiple bundles for local policies
	return []*generic.BundleEntry{
		generic.NewBundleEntry(localComplianceTransportKey,
			localComplianceBundle, localPolicyStatusPredicate),
		generic.NewBundleEntry(localCompleteComplianceTransportKey,
			localCompleteComplianceBundle, localPolicyStatusPredicate),
		generic.NewBundleEntry(localPolicySpecTransportKey, localPolicySpecBundle,
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
