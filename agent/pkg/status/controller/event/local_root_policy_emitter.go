package event

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.ObjectEmitter = &localRootPolicyEmitter{}

type localRootPolicyEmitter struct {
	ctx             context.Context
	name            string
	log             logr.Logger
	runtimeClient   client.Client
	eventType       string
	topic           string
	currentVersion  *version.Version
	lastSentVersion version.Version
	payload         event.RootPolicyEventBundle
}

func NewLocalRootPolicyEmitter(ctx context.Context, c client.Client, topic string) *localRootPolicyEmitter {
	name := strings.Replace(string(enum.LocalRootPolicyEventType), enum.EventTypePrefix, "", -1)
	filter.RegisterTimeFilter(name)
	return &localRootPolicyEmitter{
		ctx:             ctx,
		name:            name,
		log:             ctrl.Log.WithName(name),
		eventType:       string(enum.LocalRootPolicyEventType),
		topic:           transport.GenericEventTopic,
		runtimeClient:   c,
		currentVersion:  version.NewVersion(),
		lastSentVersion: *version.NewVersion(),
		payload:         make([]event.RootPolicyEvent, 0),
	}
}

func (h *localRootPolicyEmitter) PostUpdate() {
	h.currentVersion.Incr()
}

func (h *localRootPolicyEmitter) ShouldUpdate(obj client.Object) bool {
	if config.GetEnableLocalPolicy() != config.EnableLocalPolicyTrue {
		return false
	}

	policy, ok := policyEventPredicate(h.ctx, obj, h.runtimeClient, h.log)

	return ok && !utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) &&
		!utils.HasLabel(policy, constants.PolicyEventRootPolicyNameLabelKey)
}

func policyEventPredicate(ctx context.Context, obj client.Object, c client.Client, log logr.Logger) (
	*policiesv1.Policy, bool,
) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil, false
	}

	if evt.InvolvedObject.Kind != policiesv1.Kind {
		return nil, false
	}

	// get policy
	policy, err := getInvolvePolicy(ctx, c, evt)
	if err != nil {
		log.Info("failed to get involved policy", "event", evt.Namespace+"/"+evt.Name, "error", err.Error())
		return nil, false
	}
	return policy, true
}

func (h *localRootPolicyEmitter) Update(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}
	// if it's a older event, then return false
	if !filter.Newer(h.name, evt.LastTimestamp.Time) {
		return false
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name,
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
			Count:          evt.Count,
			Source:         evt.Source,
			CreatedAt:      evt.LastTimestamp,
		},
		PolicyID:   string(policy.GetUID()),
		Compliance: policyCompliance(policy, evt),
	}
	h.payload = append(h.payload, rootPolicyEvent)
	return true
}

func (*localRootPolicyEmitter) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *localRootPolicyEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	if len(h.payload) < 1 {
		return nil, fmt.Errorf("the cloudevent instance shouldn't be nil")
	}
	e := cloudevents.NewEvent()
	e.SetType(h.eventType)
	e.SetSource(config.GetLeafHubName())
	e.SetExtension(version.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	return &e, err
}

// to assert whether emit the current cloudevent
func (h *localRootPolicyEmitter) ShouldSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *localRootPolicyEmitter) Topic() string {
	return h.topic
}

func (h *localRootPolicyEmitter) PostSend() {
	// update the time filter: with latest event
	for _, evt := range h.payload {
		filter.CacheTime(h.name, evt.CreatedAt.Time)
	}
	// update version and clean the cache
	h.payload = make([]event.RootPolicyEvent, 0)
	// 1. the version get into the next generation
	// 2. set the lastSenteVersion to current version
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}

func getEventKey(event *corev1.Event) string {
	return fmt.Sprintf("%s-%s-%d", event.Namespace, event.Name, event.Count)
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
