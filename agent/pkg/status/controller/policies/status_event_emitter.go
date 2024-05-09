package policies

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	_                          generic.ObjectEmitter = &statusEventEmitter{}
	MessageCompliaceStateRegex                       = regexp.MustCompile(`(\w+);`)
)

type statusEventEmitter struct {
	ctx             context.Context
	name            string
	log             logr.Logger
	eventType       string
	runtimeClient   client.Client
	currentVersion  *eventversion.Version
	lastSentVersion eventversion.Version
	payload         event.ReplicatedPolicyEventBundle
	topic           string
	predicate       func(client.Object) bool
}

func StatusEventEmitter(
	ctx context.Context,
	eventType enum.EventType,
	predicate func(client.Object) bool,
	c client.Client,
	topic string,
) generic.ObjectEmitter {
	name := strings.Replace(string(eventType), enum.EventTypePrefix, "", -1)
	filter.RegisterTimeFilter(name)
	return &statusEventEmitter{
		ctx:             ctx,
		name:            name,
		log:             ctrl.Log.WithName(name),
		eventType:       string(eventType),
		topic:           topic,
		runtimeClient:   c,
		currentVersion:  eventversion.NewVersion(),
		lastSentVersion: *eventversion.NewVersion(),
		payload:         make([]event.ReplicatedPolicyEvent, 0),
		predicate:       predicate,
	}
}

// replicated policy
func (h *statusEventEmitter) ShouldUpdate(obj client.Object) bool {
	return h.predicate(obj)
}

func (h *statusEventEmitter) PostUpdate() {
	h.currentVersion.Incr()
}

func (h *statusEventEmitter) ShouldSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *statusEventEmitter) Topic() string {
	return h.topic
}

func (h *statusEventEmitter) Update(obj client.Object) bool {
	policy, ok := obj.(*policiesv1.Policy)
	if !ok {
		return false // do not handle objects other than policy
	}
	if policy.Status.Details == nil {
		return false // no status to update
	}

	rootPolicy, clusterID, err := GetRootPolicyAndClusterID(h.ctx, policy, h.runtimeClient)
	if err != nil {
		h.log.Error(err, "failed to get get rootPolicy/clusterID by replicatedPolicy")
		return false
	}

	updated := false
	for _, detail := range policy.Status.Details {
		if detail.History != nil {
			for _, evt := range detail.History {
				// if the event time is older thant the filter cached sent event time, then skip it
				if !filter.Newer(h.name, evt.LastTimestamp.Time) {
					continue
				}

				h.payload = append(h.payload, event.ReplicatedPolicyEvent{
					BaseEvent: event.BaseEvent{
						EventName:      evt.EventName,
						EventNamespace: policy.Namespace,
						Message:        evt.Message,
						Reason:         "PolicyStatusSync",
						Count:          1,
						Source: corev1.EventSource{
							Component: "policy-status-history-sync",
						},
						CreatedAt: evt.LastTimestamp,
					},
					PolicyID:   string(rootPolicy.GetUID()),
					ClusterID:  clusterID,
					Compliance: GetComplianceState(MessageCompliaceStateRegex, evt.Message, string(detail.ComplianceState)),
				})
				updated = true
			}
		}
	}
	return updated
}

func (*statusEventEmitter) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *statusEventEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	if len(h.payload) < 1 {
		return nil, fmt.Errorf("the payload shouldn't be nil")
	}
	e := cloudevents.NewEvent()
	e.SetSource(config.GetLeafHubName())
	e.SetType(h.eventType)
	e.SetExtension(eventversion.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	return &e, err
}

func (h *statusEventEmitter) PostSend() {
	// update the time filter: with latest event
	for _, evt := range h.payload {
		filter.CacheTime(h.name, evt.CreatedAt.Time)
	}
	// update version and clean the cache
	h.payload = make([]event.ReplicatedPolicyEvent, 0)
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
}

func GetComplianceState(regex *regexp.Regexp, message, defaultVal string) string {
	match := regex.FindStringSubmatch(message)
	if len(match) > 1 {
		firstWord := strings.TrimSpace(match[1])
		return firstWord
	}
	return defaultVal
}

func GetRootPolicyAndClusterID(ctx context.Context, replicatedPolicy *policiesv1.Policy, c client.Client) (
	rootPolicy *policiesv1.Policy, clusterID string, err error,
) {
	rootPolicyNamespacedName := replicatedPolicy.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	rootPolicy, err = utils.GetRootPolicy(ctx, c, rootPolicyNamespacedName)
	if err != nil {
		return nil, "", err
	}

	clusterName, ok := replicatedPolicy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return rootPolicy, clusterID,
			fmt.Errorf("label %s not found in policy %s/%s",
				constants.PolicyEventClusterNameLabelKey, replicatedPolicy.Namespace, replicatedPolicy.Name)
	}
	clusterID, err = utils.GetClusterId(ctx, c, clusterName)
	return rootPolicy, clusterID, err
}
