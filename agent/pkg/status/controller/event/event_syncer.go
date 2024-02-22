package event

import (
	"context"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	statusconfig "github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

const (
	UnknownComplianceState = "Unknown"
)

var (
	PolicyMessageStatusRe = regexp.
		MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)
)

func LaunchEventSyncer(ctx context.Context, mgr ctrl.Manager,
	agentConfig *config.AgentConfig, producer transport.Producer) error {

	eventTopic := agentConfig.TransportConfig.KafkaConfig.Topics.EventTopic

	instance := func() client.Object {
		return &corev1.Event{}
	}

	predicate := predicate.NewPredicateFuncs(func(obj client.Object) bool {
		event, ok := obj.(*corev1.Event)
		if !ok {
			return false
		}
		// only sync the policy event || extend other InvolvedObject kind
		return event.InvolvedObject.Kind == policiesv1.Kind
	})

	return generic.LaunchGenericObjectSyncer(
		"status.event",
		mgr,
		generic.NewGenericController(instance, predicate),
		producer,
		statusconfig.GetEventDuration,
		[]generic.ObjectEmitter{
			NewLocalRootPolicyEmitter(ctx, mgr.GetClient(), eventTopic),
		})
}
