package policies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/policies/handlers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/grc"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	log             = logger.DefaultZapLogger()
	runtimeClient   client.Client
	addPolicySyncer = false
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

func AddPolicySyncer(ctx context.Context, mgr ctrl.Manager, p transport.Producer,
	periodicSyncer *generic.PeriodicSyncer, agentConfig *configs.AgentConfig,
) error {
	runtimeClient = mgr.GetClient()
	if addPolicySyncer {
		return nil
	}
	// 1. emitter: define how to load the object into bundle and send the bundle
	// 1.1 local policy spec
	localPolicySpecEmitter := emitters.NewObjectEmitter(
		enum.LocalPolicySpecType,
		p,
		emitters.WithPredicateFunc(localPolicySpecPredicate),
		emitters.WithTargetFunc(enableLocalRootPolicy),
		emitters.WithTweakFunc(localPolicySpecTweakFunc),
	)

	// 1.2 local replicated policy status events
	localReplicatedPolicyEventEmitter := emitters.NewEventEmitter(
		enum.LocalReplicatedPolicyEventType,
		p,
		runtimeClient,
		enableLocalReplicatedPolicy,
		localReplicatedPolicyEventTransform,
		emitters.WithPostSend(localReplicatedPolicyEventPostSend),
	)

	// 2. add the emitter to controller
	if err := generic.AddSyncCtrl(
		mgr,
		"localpolicy",
		func() client.Object { return &policiesv1.Policy{} },
		localPolicySpecEmitter,
		localReplicatedPolicyEventEmitter,
	); err != nil {
		return err
	}

	// 3. register the emitter to periodic syncer
	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: listTargetPolicies(enableLocalRootPolicy),
		Emitter:  localPolicySpecEmitter,
	})

	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: listTargetPolicies(enableLocalReplicatedPolicy),
		Emitter:  localReplicatedPolicyEventEmitter,
	})

	addPolicySyncer = true
	return nil
}
