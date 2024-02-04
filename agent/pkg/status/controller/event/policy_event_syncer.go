package event

import (
	"fmt"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
)

const (
	CloudEventTypePrefix   = "io.open-cluster-management.operator.multiclusterglobalhubs"
	UnknownComplianceState = "Unknown"
)

var (
	PolicyMessageStatusRe = regexp.MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)

	LocalReplicatedPolicyEventType = fmt.Sprintf("%s.local.replicatedpolicy.update", CloudEventTypePrefix)
	LocalRootPolicyEventType       = fmt.Sprintf("%s.local.policy.propagate", CloudEventTypePrefix)
)
var _ generic.ObjectSyncer = &policyEventSyncer{}

type policyEventSyncer struct {
	name      string
	interval  func() time.Duration
	finalizer bool
	topic     string
}

func NewPolicyEventSyncer() *policyEventSyncer {
	return &policyEventSyncer{
		name:      "policyevent-syncer",
		interval:  config.GetEventDuration,
		finalizer: false,
		topic:     "event",
	}
}

func (s *policyEventSyncer) Name() string {
	return s.name
}

func (s *policyEventSyncer) Instance() client.Object {
	return &corev1.Event{}
}

func (s *policyEventSyncer) Predicate() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event
		return event.InvolvedObject.Kind == "Policy"
	})
}

func (s *policyEventSyncer) Interval() func() time.Duration {
	return s.interval
}

func (s *policyEventSyncer) EnableFinalizer() bool {
	return s.finalizer
}

func (s *policyEventSyncer) Topic() string {
	return s.topic
}
