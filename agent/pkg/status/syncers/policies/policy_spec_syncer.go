package policies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var addedPolicySpecSyncer = false

func AddPolicySpecSyncer(ctx context.Context, mgr ctrl.Manager, p transport.Producer,
	periodicSyncer *generic.PeriodicSyncer,
) error {
	if addedPolicySpecSyncer {
		return nil
	}
	// 1. emitter: define how to load the object into bundle and send the bundle
	enableLocalPolicy := func(obj client.Object) bool {
		return configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue && // enable local policy
			!utils.HasAnnotation(obj, constants.OriginOwnerReferenceAnnotation) && // local resource
			!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
	}
	localPolicySpecEmitter := emitters.NewObjectEmitter(
		enum.LocalPolicySpecType,
		p,
		emitters.WithEventFilterFunc(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return enableLocalPolicy(e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return enableLocalPolicy(e.ObjectNew) && e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return enableLocalPolicy(e.Object)
			},
		}),
		emitters.WithTweakFunc(func(object client.Object) {
			policy, ok := object.(*policiesv1.Policy)
			if !ok {
				panic("Wrong instance passed to clean policy function, not a Policy")
			}
			policy.Status = policiesv1.PolicyStatus{}
		}),
	)

	// 2. add the emitter to controller
	if err := generic.AddSyncCtrl(
		mgr,
		func() client.Object { return &policiesv1.Policy{} },
		localPolicySpecEmitter,
	); err != nil {
		return err
	}

	// 3. register the emitter to periodic syncer
	periodicSyncer.Register(&generic.EmitterRegistration{
		ListFunc: func() ([]client.Object, error) {
			var policies policiesv1.PolicyList
			if err := mgr.GetClient().List(ctx, &policies); err != nil {
				return nil, err
			}
			var filtered []client.Object
			for i := range policies.Items {
				obj := &policies.Items[i]
				if enableLocalPolicy(obj) {
					filtered = append(filtered, obj)
				}
			}
			return filtered, nil
		},
		Emitter: localPolicySpecEmitter,
	})

	addedPolicySpecSyncer = true
	return nil
}
