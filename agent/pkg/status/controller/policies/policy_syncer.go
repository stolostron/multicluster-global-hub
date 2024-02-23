package policies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func LaunchPolicySyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *config.AgentConfig,
	producer transport.Producer) error {

	// controller config
	instance := func() client.Object { return &policiesv1.Policy{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })
	controller := generic.NewGenericController(instance, predicate)

	// emitters
	// 1. local compliance
	localComplianceVersion := metadata.NewBundleVersion()
	localCompliancePredicate := func(obj client.Object) bool {
		return statusconfig.GetAggregationLevel() == statusconfig.AggregationFull && // full level
			statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue && // enable local policy
			!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
			!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	}
	localComplianceEmitter := ComplianceEmitterWrapper(
		enum.LocalPolicyComplianceType,
		localComplianceVersion,
		localCompliancePredicate,
	)

	// 2. local complete compliance
	localCompleteEmitter := CompleteComplianceEmitterWrapper(
		enum.LocalPolicyCompleteComplianceType,
		localComplianceVersion,
		localCompliancePredicate,
	)

	// 3. local policy event
	eventTopic := agentConfig.TransportConfig.KafkaConfig.Topics.EventTopic
	localStatusEventEmitter := StatusEventEmitter(ctx, enum.LocalReplicatedPolicyEventType,
		func(obj client.Object) bool {
			return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue &&
				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
				utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // replicated policy
		},
		mgr.GetClient(),
		eventTopic,
	)

	// 4. local policy spec
	localPolicySpecEmitter := generic.ObjectEmitterWrapper(enum.LocalPolicySpecType,
		func(obj client.Object) bool {
			return statusconfig.GetEnableLocalPolicy() == statusconfig.EnableLocalPolicyTrue && // enable local policy
				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
				!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
		},
		cleanPolicy,
	)

	// global policy emitters
	// 5. global compliance
	complianceVersion := metadata.NewBundleVersion()
	compliancePredicate := func(obj client.Object) bool {
		return statusconfig.GetAggregationLevel() == statusconfig.AggregationFull && // full level
			utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // global resource
			!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	}
	complianceEmitter := ComplianceEmitterWrapper(
		enum.PolicyComplianceType,
		complianceVersion,
		compliancePredicate,
	)

	// 6. global complete compliance
	completeEmitter := CompleteComplianceEmitterWrapper(
		enum.PolicyCompleteComplianceType,
		complianceVersion,
		compliancePredicate,
	)

	return generic.LaunchGenericObjectSyncer(
		"status.policy",
		mgr,
		controller,
		producer,
		statusconfig.GetPolicyDuration,
		[]generic.ObjectEmitter{
			localComplianceEmitter,
			localCompleteEmitter,
			localStatusEventEmitter,
			localPolicySpecEmitter,
			// global compliance
			complianceEmitter,
			completeEmitter,
		})
}

func cleanPolicy(object client.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean policy function, not a Policy")
	}
	policy.Status = policiesv1.PolicyStatus{}
}
