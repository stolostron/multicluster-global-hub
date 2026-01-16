package policies

import (
	"context"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func enableLocalRootPolicy(obj client.Object) bool {
	return configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue && // enable local policy
		!utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // root policy
}

var localPolicySpecPredicate = predicate.Funcs{
	CreateFunc: func(e event.CreateEvent) bool {
		return enableLocalRootPolicy(e.Object)
	},
	UpdateFunc: func(e event.UpdateEvent) bool {
		return enableLocalRootPolicy(e.ObjectNew) && e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration()
	},
	DeleteFunc: func(e event.DeleteEvent) bool {
		return enableLocalRootPolicy(e.Object)
	},
}

var localPolicySpecTweakFunc = func(object client.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		log.Errorf("Wrong instance passed to clean policy function, not a Policy")
	}
	policy.SetManagedFields(nil)
	policy.Status = policiesv1.PolicyStatus{}
}

func listTargetPolicies(target func(client.Object) bool) func() ([]client.Object, error) {
	return func() ([]client.Object, error) {
		var policies policiesv1.PolicyList
		if err := runtimeClient.List(context.Background(), &policies); err != nil {
			return nil, err
		}
		var filtered []client.Object
		for i := range policies.Items {
			obj := &policies.Items[i]
			if target(obj) {
				filtered = append(filtered, obj)
			}
		}
		return filtered, nil
	}
}
