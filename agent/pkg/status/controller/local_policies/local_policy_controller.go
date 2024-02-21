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

var _ generic.ObjectController = &localPolicyController{}

type localPolicyController struct {
	interval  func() time.Duration
	finalizer bool
}

func NewLocalPolicyController() *localPolicyController {
	return &localPolicyController{
		interval:  config.GetPolicyDuration,
		finalizer: false,
	}
}

func (s *localPolicyController) Instance() client.Object {
	return &policiesv1.Policy{}
}

// enable the local policy and only for local policy
func (s *localPolicyController) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(object client.Object) bool {
		return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue &&
			!utils.HasAnnotation(object, constants.OriginOwnerReferenceAnnotation)
	})
}

func (s *localPolicyController) EnableFinalizer() bool {
	return s.finalizer
}
