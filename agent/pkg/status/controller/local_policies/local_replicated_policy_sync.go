package localpolicies

import (
	"context"
	"fmt"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

// AddLocalPoliciesController this function adds a new local policies sync controller.
func AddLocalReplicatedPolicySyncer(mgr ctrl.Manager, producer transport.Producer) error {
	createObjFunc := func() bundle.Object { return &policiesv1.Policy{} }
	leafHubName := config.GetLeafHubName()

	localClusterPolicyHistoryEventTransportKey := fmt.Sprintf("%s.%s", leafHubName,
		constants.LocalPolicyHistoryEventMsgKey)
	localPolicyHistoryEventBundle := grc.NewAgentLocalReplicatedPolicyEventBundle(context.TODO(),
		leafHubName, mgr.GetClient())

	localClusterPolicyBundleEntryCollection := []*generic.BundleEntry{
		generic.NewBundleEntry(localClusterPolicyHistoryEventTransportKey, localPolicyHistoryEventBundle,
			func() bool { return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue }),
	}

	localClusterPolicyPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			utils.HasLabel(object, rootPolicyLabel)
	})

	return generic.NewGenericStatusSyncer(mgr, "local-replicas-policies-status-sync", producer,
		localClusterPolicyBundleEntryCollection, createObjFunc, localClusterPolicyPredicate,
		config.GetPolicyDuration)
}
