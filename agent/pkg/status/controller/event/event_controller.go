package event

import (
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
)

const (
	UnknownComplianceState = "Unknown"
)

var (
	_                     generic.ObjectController = &eventController{}
	PolicyMessageStatusRe                          = regexp.
				MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)
)

type eventController struct {
	name      string
	interval  func() time.Duration
	finalizer bool
}

func NewEventController() *eventController {
	return &eventController{
		name:      "policy-event-syncer",
		interval:  config.GetEventDuration,
		finalizer: false,
	}
}

func (s *eventController) Name() string {
	return s.name
}

func (s *eventController) Instance() client.Object {
	return &corev1.Event{}
}

func (s *eventController) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event || extend other InvolvedObject kind
		return event.InvolvedObject.Kind == policiesv1.Kind
	})
}

func (s *eventController) Interval() func() time.Duration {
	return s.interval
}

func (s *eventController) EnableFinalizer() bool {
	return s.finalizer
}
