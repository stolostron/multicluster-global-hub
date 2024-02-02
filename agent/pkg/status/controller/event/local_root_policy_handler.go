package event

import (
	"context"
	"fmt"
	"regexp"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	RootPolicyLabel        = "policy.open-cluster-management.io/root-policy"
	UnknownComplianceState = "Unknown"
	RootEventType          = "io.open-cluster-management.operator.multiclusterglobalhubs.policy.propagate"
)

var PolicyMessageStatusRe = regexp.MustCompile(`Policy (.+) status was updated to (.+) in cluster namespace (.+)`)

type rootPolicyEventHandler struct {
	ctx           context.Context
	log           logr.Logger
	runtimeClient client.Client
	version       *metadata.BundleVersion
	events        map[string]*event.RootPolicyEvent
}

func NewRootPolicyEventHandler(ctx context.Context, runtimeClient client.Client) generic.ObjectHandler {
	return &rootPolicyEventHandler{
		ctx:           ctx,
		log:           ctrl.Log.WithName("root-policy-event"),
		runtimeClient: runtimeClient,
		version:       metadata.NewBundleVersion(),
	}
}

func (h *rootPolicyEventHandler) Update(obj client.Object) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	// if exist, then return
	evtKey := getEventKey(evt)
	if _, ok = h.events[evtKey]; ok {
		return
	}

	// get policy
	policy := &policiesv1.Policy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evt.InvolvedObject.Name,
			Namespace: evt.InvolvedObject.Namespace,
		},
	}
	err := h.runtimeClient.Get(h.ctx, client.ObjectKeyFromObject(policy), policy)
	if errors.IsNotFound(err) {
		h.log.Error(err, "failed to get involved object", "event", evt.Namespace+"/"+evt.Name,
			"policy", policy.Namespace+"/"+policy.Name)
		return
	}

	// global resource || replicated policy
	if utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) ||
		utils.HasLabelKey(policy.GetLabels(), RootPolicyLabel) {
		return
	}

	// update
	rootPolicyEvent := &event.RootPolicyEvent{
		BaseEvent: event.BaseEvent{
			EventName:      evt.Name,
			EventNamespace: evt.Namespace,
			Message:        evt.Message,
			Reason:         evt.Reason,
			Count:          evt.Count,
			Source:         evt.Source,
			CreatedAt:      evt.CreationTimestamp,
		},
		PolicyID:   string(policy.GetUID()),
		Compliance: policyCompliance(policy, evt),
	}
	// cache to events and update version
	h.events[evtKey] = rootPolicyEvent
	h.version.Incr()
}

func (*rootPolicyEventHandler) Delete(client.Object) {
	// do nothing
}

func (h *rootPolicyEventHandler) GetVersion() *metadata.BundleVersion {
	return h.version
}

func (h *rootPolicyEventHandler) ToCloudEvent() *cloudevents.Event {
	if len(h.events) < 1 {
		return nil
	}
	e := cloudevents.NewEvent()
	values := []event.RootPolicyEvent{}
	for _, value := range h.events {
		values = append(values, *value)
	}
	e.SetID(values[0].PolicyID)
	e.SetType(RootEventType)
	e.SetData(cloudevents.ApplicationJSON, values)
	return &e
}

func (h *rootPolicyEventHandler) PostSend() {
	// update version and clean the cache
	h.version.Next()
	for key := range h.events {
		delete(h.events, key)
	}
}

func getEventKey(event *corev1.Event) string {
	return fmt.Sprintf("%s-%s-%s", event.Namespace, event.Name, event.Count)
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
