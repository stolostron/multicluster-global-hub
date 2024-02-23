package localpolicies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func LaunchLocalPolicySyncer(ctx context.Context, mgr ctrl.Manager,
	agentConfig *config.AgentConfig, producer transport.Producer) error {

	eventTopic := agentConfig.TransportConfig.KafkaConfig.Topics.EventTopic
	return generic.LaunchGenericObjectSyncer(
		"status.local_policy",
		mgr,
		generic.NewGenericController(policyInstance, localPolicyPredicate()),
		producer,
		statusconfig.GetPolicyDuration,
		[]generic.ObjectEmitter{
			StatusEventEmitter(ctx, mgr.GetClient(), eventTopic),
		})
}

func policyInstance() client.Object {
	return &policiesv1.Policy{}
}

// enable the local policy and only for local policy
func localPolicyPredicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue &&
			!utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})
}
