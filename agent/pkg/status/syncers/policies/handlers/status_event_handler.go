package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	MessageCompliaceStateRegex = regexp.MustCompile(`(\w+);`)
	log                        = logger.DefaultZapLogger()
)

type policyStatusEventHandler struct {
	ctx           context.Context
	name          string
	runtimeClient client.Client
	payload       *event.ReplicatedPolicyEventBundle
	shouldUpdate  func(client.Object) bool
}

func NewPolicyStatusEventHandler(
	ctx context.Context,
	eventType enum.EventType,
	shouldUpdate func(client.Object) bool,
	c client.Client,
) *policyStatusEventHandler {
	name := strings.ReplaceAll(string(eventType), enum.EventTypePrefix, "")
	filter.RegisterTimeFilter(name)
	return &policyStatusEventHandler{
		ctx:           ctx,
		name:          name,
		runtimeClient: c,
		payload:       &event.ReplicatedPolicyEventBundle{},
		shouldUpdate:  shouldUpdate,
	}
}

func (h *policyStatusEventHandler) Get() interface{} {
	return h.payload
}

// only for test
func (h *policyStatusEventHandler) Append(evt event.ReplicatedPolicyEvent) {
	*h.payload = append(*h.payload, evt)
}

func (h *policyStatusEventHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	policy, ok := obj.(*policiesv1.Policy)
	if !ok {
		return false // do not handle objects other than policy
	}

	if policy.Status.Details == nil {
		return false // no status to update
	}

	rootPolicy, clusterID, clusterName, err := GetRootPolicyAndClusterInfo(h.ctx, policy, h.runtimeClient)
	if err != nil {
		log.Errorf("failed to get get rootPolicy/clusterID by replicatedPolicy: %v", err)
		return false
	}

	updated := false
	for _, detail := range policy.Status.Details {
		if detail.History != nil {
			for _, evt := range detail.History {
				// if the event time is older thant the filter cached sent event time, then skip it
				if !filter.Newer(h.name, evt.LastTimestamp.Time) {
					log.Debugf("skip the expired event: %s", evt.EventName)
					continue
				}

				*h.payload = append(*h.payload, event.ReplicatedPolicyEvent{
					BaseEvent: event.BaseEvent{
						EventName:      evt.EventName,
						EventNamespace: policy.Namespace,
						Message:        evt.Message,
						Reason:         "PolicyStatusSync",
						Count:          1,
						Source: corev1.EventSource{
							Component: "policy-status-history-sync",
						},
						CreatedAt: evt.LastTimestamp.Time,
					},
					PolicyID:    string(rootPolicy.GetUID()),
					ClusterID:   clusterID,
					ClusterName: clusterName,
					Compliance:  GetComplianceState(MessageCompliaceStateRegex, evt.Message, string(detail.ComplianceState)),
				})
				updated = true
			}
		}
	}
	return updated
}

func (*policyStatusEventHandler) Delete(client.Object) bool {
	return false
}

func NewPolicyStatusEventEmitter(eventType enum.EventType) interfaces.Emitter {
	name := strings.ReplaceAll(string(eventType), enum.EventTypePrefix, "")
	return generic.NewGenericEmitter(eventType, generic.WithPostSend(
		// After sending the event, update the filter cache and clear the bundle from the handler cache.
		func(data interface{}) {
			events, ok := data.(*event.ReplicatedPolicyEventBundle)
			if !ok {
				return
			}
			// policyEvents, ok := data.([]event.ReplicatedPolicyEvent)
			// update the time filter: with latest event
			for _, evt := range *events {
				filter.CacheTime(name, evt.CreatedAt)
			}
			// reset the payload
			*events = (*events)[:0]
		}),
	)
}

func GetComplianceState(regex *regexp.Regexp, message, defaultVal string) string {
	match := regex.FindStringSubmatch(message)
	if len(match) > 1 {
		firstWord := strings.TrimSpace(match[1])
		return firstWord
	}
	return defaultVal
}

func GetRootPolicyAndClusterInfo(ctx context.Context, replicatedPolicy *policiesv1.Policy, c client.Client) (
	rootPolicy *policiesv1.Policy, clusterID, clusterName string, err error,
) {
	rootPolicyNamespacedName := replicatedPolicy.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	rootPolicy, err = utils.GetRootPolicy(ctx, c, rootPolicyNamespacedName)
	if err != nil {
		return nil, "", "", err
	}

	clusterName, ok := replicatedPolicy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return rootPolicy, "", "",
			fmt.Errorf("label %s not found in policy %s/%s",
				constants.PolicyEventClusterNameLabelKey, replicatedPolicy.Namespace, replicatedPolicy.Name)
	}
	clusterID, err = utils.GetClusterId(ctx, c, clusterName)
	return rootPolicy, clusterID, clusterName, err
}
