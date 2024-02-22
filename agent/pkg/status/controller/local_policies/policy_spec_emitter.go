package localpolicies

import (
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	genericpayload "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func NewPolicySpecEmitter(topic string) generic.ObjectEmitter {
	predicate := func(obj client.Object) bool {
		return !utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey)
	}
	eventData := genericpayload.GenericObjectData{}
	return generic.NewGenericObjectEmitter(
		enum.LocalPolicySpecType,
		eventData,
		generic.NewGenericObjectHandler(eventData),
		generic.WithTopic(topic),
		generic.WithPredicate(predicate),
		generic.WithTweakFunc(cleanPolicy),
	)
}

// status will be sent in the policy status bundles.
func cleanPolicy(object client.Object) {
	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		panic("Wrong instance passed to clean policy function, not a Policy")
	}
	policy.Status = policiesv1.PolicyStatus{}
}
