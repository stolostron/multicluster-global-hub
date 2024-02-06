package event

import (
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
)

const (
	UnknownComplianceState = "Unknown"
)

var (
	_                     generic.ObjectSyncer = &eventSyncer{}
	PolicyMessageStatusRe                      = regexp.
				MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)
)

type eventSyncer struct {
	name      string
	interval  func() time.Duration
	finalizer bool
}

func NewEventSyncer() *eventSyncer {
	return &eventSyncer{
		name:      "policy-event-syncer",
		interval:  config.GetEventDuration,
		finalizer: false,
	}
}

func (s *eventSyncer) Name() string {
	return s.name
}

func (s *eventSyncer) Instance() client.Object {
	return &corev1.Event{}
}

func (s *eventSyncer) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event
		return event.InvolvedObject.Kind == "Policy"
	})
}

func (s *eventSyncer) Interval() func() time.Duration {
	return s.interval
}

func (s *eventSyncer) EnableFinalizer() bool {
	return s.finalizer
}
