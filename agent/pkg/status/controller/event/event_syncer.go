package event

import (
	"time"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/operator/pkg/transporter"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type ObjectSyncer interface {
	Instance() generic.Object
	Predicate() predicate.Predicate
	Interval() func() time.Duration
	EnableFinalizer() bool
	Topic() string
}

type eventSyncer struct{}

func NewEventTopic() *eventSyncer {
	return &eventSyncer{}
}

func (*eventSyncer) Instance() generic.Object {
	return &corev1.Event{}
}

func (*eventSyncer) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event
		return event.InvolvedObject.Kind == "Policy"
	})
}

func (*eventSyncer) Interval() func() time.Duration {
	return config.GetEventDuration
}

func (*eventSyncer) EnableFinalizer() bool {
	return false
}

func (*eventSyncer) Topic() string {
	return transporter.GenericEventTopic
}
