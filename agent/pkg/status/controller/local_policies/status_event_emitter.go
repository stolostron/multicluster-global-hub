package localpolicies

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	lru "github.com/hashicorp/golang-lru"
	corev1 "k8s.io/api/core/v1"
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

var (
	_                          generic.EventEmitter = &statusEventEmitter{}
	MessageCompliaceStateRegex                      = regexp.MustCompile(`(\w+);`)
)

type statusEventEmitter struct {
	ctx             context.Context
	log             logr.Logger
	eventType       string
	runtimeClient   client.Client
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
	events          []event.ReplicatedPolicyEvent
	cache           *lru.Cache
	topic           string
}

func StatusEventEmitter(ctx context.Context, runtimeClient client.Client) generic.EventEmitter {
	cache, _ := lru.New(30)
	return &statusEventEmitter{
		ctx:             ctx,
		log:             ctrl.Log.WithName("local-replicated-policy-syncer/status-event"),
		eventType:       string(enum.LocalReplicatedPolicyEventType),
		topic:           transport.GenericEventTopic,
		runtimeClient:   runtimeClient,
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
		cache:           cache,
		events:          make([]event.ReplicatedPolicyEvent, 0),
	}
}

func (h *statusEventEmitter) Emit() bool {
	return config.GetEnableLocalPolicy() == config.EnableLocalPolicyTrue && h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *statusEventEmitter) Topic() string {
	return h.topic
}

func (h *statusEventEmitter) Update(obj client.Object) {
	policy, ok := obj.(*policiesv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}
	if policy.Status.Details == nil {
		return // no status to update
	}

	rootPolicyNamespacedName := policy.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	rootPolicy, err := utils.GetRootPolicy(h.ctx, h.runtimeClient, rootPolicyNamespacedName)
	if err != nil {
		h.log.Error(err, "failed to get root policy", "namespacedName", rootPolicyNamespacedName)
		return
	}

	clusterName, ok := policy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		h.log.Error(errors.New("cluster name not found in replicated policy"), "policy", policy.Namespace+"/"+policy.Name)
		return
	}
	clusterId, err := utils.GetClusterId(h.ctx, h.runtimeClient, clusterName)
	if err != nil {
		h.log.Error(err, "failed to get cluster id by cluster", "clusterName", clusterName)
		return
	}

	updated := false
	for _, detail := range policy.Status.Details {
		if detail.History != nil {
			for _, evt := range detail.History {
				key := fmt.Sprintf("%s.%s", evt.EventName, evt.LastTimestamp)
				if h.cache.Contains(key) {
					continue
				}

				h.events = append(h.events, event.ReplicatedPolicyEvent{
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
					ClusterID:  clusterId,
					Compliance: GetComplianceState(MessageCompliaceStateRegex, evt.Message, string(detail.ComplianceState)),
				})
				h.cache.Add(key, nil)
				updated = true
			}
		}
	}
	if updated {
		h.currentVersion.Incr()
	}
}

func (*statusEventEmitter) Delete(client.Object) {
	// do nothing
}

func (h *statusEventEmitter) ToCloudEvent() *cloudevents.Event {
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

func (h *statusEventEmitter) PostSend() {
	// update version and clean the cache
	h.events = make([]event.ReplicatedPolicyEvent, 0)
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
