package localpolicies

import (
	"time"

	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.ObjectSyncer = &localPolicySyncer{}

type localPolicySyncer struct {
	name      string
	interval  func() time.Duration
	finalizer bool
}

func NewLocalPolicySyncer() *localPolicySyncer {
	return &localPolicySyncer{
		name:      "local-replicated-policy-syncer",
		interval:  config.GetPolicyDuration,
		finalizer: false,
	}
}

func (s *localPolicySyncer) Name() string {
	return s.name
}

func (s *localPolicySyncer) Instance() client.Object {
	return &policiesv1.Policy{}
}

func (s *localPolicySyncer) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return !utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation) &&
			utils.HasLabelKey(object.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey)
	})
}

func (s *localPolicySyncer) Interval() func() time.Duration {
	return s.interval
}

func (s *localPolicySyncer) EnableFinalizer() bool {
	return s.finalizer
}
