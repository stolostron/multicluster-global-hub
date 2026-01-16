package events

import (
	"context"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	UnknownComplianceState = "Unknown"
)

var (
	PolicyMessageStatusRe = regexp.
				MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)
	TimeFilterKeyForLocalRootPolicy = enum.ShortenEventType(string(enum.LocalRootPolicyEventType))
)

func localRootPolicyPostSend(events []interface{}) error {
	for _, policyEvent := range events {
		evt, ok := policyEvent.(*event.RootPolicyEvent)
		if !ok {
			return fmt.Errorf("failed to type assert to event.RootPolicyEvent, event: %v", policyEvent)
		}
		filter.CacheTime(TimeFilterKeyForLocalRootPolicy, evt.CreatedAt)
	}
	return nil
}

// localRootPolicyEventPredicate filters events for LocalRootPolicy resources
func localRootPolicyEventPredicate(obj client.Object) bool {
	if configmap.GetEnableLocalPolicy() != configmap.EnableLocalPolicyTrue {
		return false
	}

	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	if evt.InvolvedObject.Kind != constants.PolicyKind {
		return false
	}

	if !filter.Newer(TimeFilterKeyForLocalRootPolicy, getEventLastTime(evt).Time) {
		log.Debugw("event filtered:", "event", evt.Namespace+"/"+evt.Name, "eventTime", getEventLastTime(evt).Time)
		return false
	}

	policy, err := getInvolvePolicy(context.Background(), runtimeClient, evt)
	if err != nil {
		log.Debugf("failed to get involved policy event: %s/%s, error: %v", evt.Namespace, evt.Name, err)
		return false
	}

	// root policy
	return !utils.HasLabel(policy, constants.PolicyEventRootPolicyNameLabelKey)
}

// localRootPolicyEventTransform transforms k8s Event to RootPolicyEvent
func localRootPolicyEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	policy, err := getInvolvePolicy(context.Background(), runtimeClient, evt)
	if err != nil {
		log.Warnf("failed to get involved policy event: %s/%s, error: %v", evt.Namespace, evt.Name, err)
		return nil
	}
	// update
	return &event.RootPolicyEvent{
		BaseEvent: event.BaseEvent{
			EventName:      evt.Name,
			EventNamespace: evt.Namespace,
			Message:        evt.Message,
			Reason:         evt.Reason,
			Count:          getEventCount(evt),
			Source:         evt.Source,
			CreatedAt:      getEventLastTime(evt).Time,
		},
		PolicyID:   string(policy.GetUID()),
		Compliance: policyCompliance(policy, evt),
	}
}

// policyCompliance extracts compliance from policy or event message
// Uses same logic as handler
func policyCompliance(policy *policyv1.Policy, evt *corev1.Event) string {
	compliance := UnknownComplianceState
	if policy.Status.ComplianceState != "" {
		compliance = string(policy.Status.ComplianceState)
	}
	if compliance != UnknownComplianceState {
		return compliance
	}

	matches := PolicyMessageStatusRe.FindStringSubmatch(evt.Message)
	if len(matches) == 4 {
		compliance = matches[2]
	}
	return compliance
}

// getEventCount gets the event count
// Uses same logic as handler
func getEventCount(evt *corev1.Event) int32 {
	count := evt.Count
	if evt.Series != nil {
		count = evt.Series.Count
	}
	return count
}

func getInvolvePolicy(ctx context.Context, c client.Client, evt *corev1.Event) (*policyv1.Policy, error) {
	policy := &policyv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evt.InvolvedObject.Name,
			Namespace: evt.InvolvedObject.Namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(policy), policy)
	return policy, err
}
