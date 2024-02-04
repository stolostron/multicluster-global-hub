package event

import (
	"context"
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	lru "github.com/hashicorp/golang-lru"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.EventEmitter = &localRootPolicyEmitter{}

type localRootPolicyEmitter struct {
	ctx             context.Context
	log             logr.Logger
	runtimeClient   client.Client
	eventType       string
	topic           string
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
	cache           *lru.Cache
	events          []event.RootPolicyEvent
}

func NewLocalRootPolicyEmitter(ctx context.Context, c client.Client) *localRootPolicyEmitter {
	cache, _ := lru.New(20)
	return &localRootPolicyEmitter{
		ctx:             ctx,
		log:             ctrl.Log.WithName("policy-event-sycner/local-root-policy"),
		eventType:       string(enum.LocalRootPolicyEventType),
		topic:           transport.GenericEventTopic,
		runtimeClient:   c,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
		cache:           cache,
		events:          make([]event.RootPolicyEvent, 0),
	}
}

func (h *localRootPolicyEmitter) Update(obj client.Object) {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return
	}
	// if exist, then return
	evtKey := getEventKey(evt)
	if ok = h.cache.Contains(evtKey); ok {
		return
	}

	// get policy
	policy, err := getInvolvePolicy(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved policy", "event", evt.Namespace+"/"+evt.Name)
	}

	// global resource || replicated policy
	if utils.HasAnnotation(policy, constants.OriginOwnerReferenceAnnotation) ||
		utils.HasLabelKey(policy.GetLabels(), constants.PolicyEventRootPolicyNameLabelKey) {
		return
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
			CreatedAt:      evt.CreationTimestamp,
		},
		PolicyID:   string(policy.GetUID()),
		Compliance: policyCompliance(policy, evt),
	}
	// cache to events and update version
	h.events = append(h.events, rootPolicyEvent)
	h.cache.Add(evtKey, nil)
	h.currentVersion.Incr()
}

func (*localRootPolicyEmitter) Delete(client.Object) {
	// do nothing
}

func (h *localRootPolicyEmitter) ToCloudEvent() *cloudevents.Event {
	if len(h.events) < 1 {
		return nil
	}
	e := cloudevents.NewEvent()
	e.SetType(h.eventType)
	e.SetExtension(metadata.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.events)
	if err != nil {
		h.log.Error(err, "failed to set the payload to cloudvents.Data")
	}
	return &e
}

// to assert whether emit the current cloudevent
func (h *localRootPolicyEmitter) Emit() bool {
	return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue && h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *localRootPolicyEmitter) Topic() string {
	return h.topic
}

func (h *localRootPolicyEmitter) PostSend() {
	// update version and clean the cache
	h.events = make([]event.RootPolicyEvent, 0)
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
