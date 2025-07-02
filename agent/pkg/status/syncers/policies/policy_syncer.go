package policies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/policies/handlers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func LaunchPolicySyncer(ctx context.Context, mgr ctrl.Manager, agentConfig *configs.AgentConfig,
	producer transport.Producer,
) error {
	// controller config
	instance := func() client.Object { return &policiesv1.Policy{} }
	predicate := predicate.NewPredicateFuncs(func(object client.Object) bool { return true })

	// 1. local compliance
	localComplianceVersion := eventversion.NewVersion()
	localComplianceShouldUpdate := func(obj client.Object) bool {
		return configmap.GetAggregationLevel() == configmap.AggregationFull && // full level
			configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue && // enable local policy
			!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
			!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	}
	localComplianceHandler := handlers.NewComplianceHandler(&grc.ComplianceBundle{}, localComplianceShouldUpdate)
	localComplianceEmitter := generic.NewGenericEmitter(enum.LocalComplianceType,
		generic.WithVersion(localComplianceVersion))

	// 2. local complete compliance
	localCompleteHandler := handlers.NewCompleteComplianceHandler(&grc.CompleteComplianceBundle{},
		localComplianceShouldUpdate)
	localCompleteEmitter := generic.NewGenericEmitter(enum.LocalCompleteComplianceType,
		generic.WithDependencyVersion(localComplianceVersion))

	// 3. local policy event
	localStatusEventHandler := handlers.NewPolicyStatusEventHandler(ctx, enum.LocalReplicatedPolicyEventType,
		// should update
		func(obj client.Object) bool {
			return configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue &&
				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
				utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // replicated policy
		},
		mgr.GetClient(),
	)
	localStatusEventEmitter := handlers.NewPolicyStatusEventEmitter(enum.LocalReplicatedPolicyEventType)

	// 4. local policy spec
	// localPolicySpecHandler := generic.NewGenericHandler(&genericpayload.GenericObjectBundle{},
	// 	generic.WithShouldUpdate(
	// 		func(obj client.Object) bool {
	// 			return configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue && // enable local policy
	// 				!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
	// 				!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	// 		}),
	// 	generic.WithSpec(true),
	// 	generic.WithTweakFunc(cleanPolicy),
	// )
	// localPolicySpecEmitter := generic.NewGenericEmitter(enum.LocalPolicySpecType)

	// 5. global policy compliance
	complianceVersion := eventversion.NewVersion()
	complianceShouldUpdate := func(obj client.Object) bool {
		return configmap.GetAggregationLevel() == configmap.AggregationFull && // full level
			utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // global resource
			!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	}
	globalComplianceHandler := handlers.NewComplianceHandler(&grc.ComplianceBundle{}, complianceShouldUpdate)
	globalComplianceEmitter := generic.NewGenericEmitter(enum.ComplianceType, generic.WithVersion(complianceVersion))

	// 6. global complete compliance
	globalCompleteHandler := handlers.NewCompleteComplianceHandler(&grc.CompleteComplianceBundle{},
		complianceShouldUpdate)
	globalCompleteEmitter := generic.NewGenericEmitter(enum.CompleteComplianceType,
		generic.WithDependencyVersion(complianceVersion))

	return generic.LaunchMultiEventSyncer(
		"status.policy",
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		configmap.GetPolicyDuration,
		[]*generic.EmitterHandler{
			{
				Handler: localComplianceHandler,
				Emitter: localComplianceEmitter,
			},
			{
				Handler: localCompleteHandler,
				Emitter: localCompleteEmitter,
			},

			{
				Handler: localStatusEventHandler,
				Emitter: localStatusEventEmitter,
			},

			// global
			{
				Handler: globalComplianceHandler,
				Emitter: globalComplianceEmitter,
			},
			{
				Handler: globalCompleteHandler,
				Emitter: globalCompleteEmitter,
			},
		})
}
