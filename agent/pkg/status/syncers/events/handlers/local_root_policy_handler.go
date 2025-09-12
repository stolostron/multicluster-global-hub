package handlers

import (
	"context"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

const (
	UnknownComplianceState = "Unknown"
)

var PolicyMessageStatusRe = regexp.
	MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)

func NewRootPolicyEventEmitter(eventType enum.EventType) interfaces.Emitter {
	name := strings.ReplaceAll(string(eventType), enum.EventTypePrefix, "")
	return generic.NewGenericEmitter(eventType, generic.WithPostSend(
		// After sending the event, update the filter cache and clear the bundle from the handler cache.
		func(data interface{}) {
			events, ok := data.(*event.RootPolicyEventBundle)
			if !ok {
				return
			}
			// update the time filter: with latest event
			for _, evt := range *events {
				filter.CacheTime(name, evt.CreatedAt)
			}
			// reset the payload
			*events = (*events)[:0]
		}),
	)
}

var _ interfaces.Handler = &localRootPolicyHandler{}

type localRootPolicyHandler struct {
	ctx           context.Context
	name          string
	runtimeClient client.Client
	eventType     string
	payload       *event.RootPolicyEventBundle
}

func NewLocalRootPolicyEventHandler(ctx context.Context, c client.Client) *localRootPolicyHandler {
	name := strings.ReplaceAll(string(enum.LocalRootPolicyEventType), enum.EventTypePrefix, "")
	filter.RegisterTimeFilter(name)
	return &localRootPolicyHandler{
		ctx:           ctx,
		name:          name,
		eventType:     string(enum.LocalRootPolicyEventType),
		runtimeClient: c,
		payload:       &event.RootPolicyEventBundle{},
	}
}

func (h *localRootPolicyHandler) Get() interface{} {
	return h.payload
}

func (h *localRootPolicyHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name,
			"policy", evt.InvolvedObject.Namespace+"/"+evt.InvolvedObject.Name)
		return false
	}

	// update
	rootPolicyEvent := event.RootPolicyEvent{
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
	*h.payload = append(*h.payload, &rootPolicyEvent)
	return true
}

func (*localRootPolicyHandler) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *localRootPolicyHandler) shouldUpdate(obj client.Object) bool {
	if configmap.GetEnableLocalPolicy() != configmap.EnableLocalPolicyTrue {
		return false
	}

	policy, ok := policyEventPredicate(h.ctx, h.name, obj, h.runtimeClient)

	return ok && !utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) &&
		!utils.HasLabel(policy, constants.PolicyEventRootPolicyNameLabelKey)
}

func policyEventPredicate(ctx context.Context, name string, obj client.Object, c client.Client) (
	*policiesv1.Policy, bool,
) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil, false
	}

	if !filter.Newer(name, getEventLastTime(evt).Time) {
		return nil, false
	}

	if evt.InvolvedObject.Kind != policiesv1.Kind {
		return nil, false
	}

	// get policy
	policy, err := getInvolvePolicy(ctx, c, evt)
	if err != nil {
		log.Debugf("failed to get involved policy event: %s/%s, error: %v", evt.Namespace, evt.Name, err)
		return nil, false
	}
	return policy, true
}

func policyCompliance(policy *policiesv1.Policy, evt *corev1.Event) string {
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

func getInvolvePolicy(ctx context.Context, c client.Client, evt *corev1.Event) (*policiesv1.Policy, error) {
	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evt.InvolvedObject.Name,
			Namespace: evt.InvolvedObject.Namespace,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(policy), policy)
	return policy, err
}

// the client-go event: https://github.com/kubernetes/client-go/blob/master/tools/events/event_recorder.go#L91-L113
// the library-go event: https://github.com/openshift/library-go/blob/master/pkg/operator/events/recorder.go#L221-L237
func getEventLastTime(evt *corev1.Event) metav1.Time {
	lastTime := evt.CreationTimestamp
	if !evt.LastTimestamp.IsZero() {
		lastTime = evt.LastTimestamp
	}
	if evt.Series != nil {
		lastTime = metav1.Time(evt.Series.LastObservedTime)
	}
	return lastTime
}

func getEventCount(evt *corev1.Event) int32 {
	count := evt.Count
	if evt.Series != nil {
		count = evt.Series.Count
	}
	return count
}
