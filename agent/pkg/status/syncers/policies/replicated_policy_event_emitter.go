package policies

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	corev1 "k8s.io/api/core/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var (
	MessageComplianceStateRegex                = regexp.MustCompile(`(\w+);`)
	TimeFilterKeyForLocalReplicatedPolicyEvent = enum.ShortenEventType(string(enum.LocalReplicatedPolicyEventType))
)

func enableLocalReplicatedPolicy(obj client.Object) bool {
	return configmap.GetEnableLocalPolicy() == configmap.EnableLocalPolicyTrue && // enable local policy
		utils.HasLabel(obj, constants.PolicyEventRootPolicyNameLabelKey) // replicated policy
}

func localReplicatedPolicyEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	clusterPolicy, ok := obj.(*policiesv1.Policy)
	if !ok {
		return nil
	}
	if clusterPolicy.Status.Details == nil {
		return nil // no status to update
	}

	rootPolicy, clusterID, clusterName, err := GetRootPolicyAndClusterInfo(context.Background(),
		clusterPolicy, runtimeClient)
	if err != nil {
		log.Warnw("failed to get replicated policy info", "error", err)
		return nil
	}

	var events []*event.ReplicatedPolicyEvent
	for _, detail := range clusterPolicy.Status.Details {
		if detail.History != nil {
			for _, evt := range detail.History {
				// if the event time is older thant the filter cached sent event time, then skip it
				if !filter.Newer(TimeFilterKeyForLocalReplicatedPolicyEvent, evt.LastTimestamp.Time) {
					log.Debugf("skip the expired replicated policy event: %s", evt.EventName)
					continue
				}

				events = append(events, &event.ReplicatedPolicyEvent{
					BaseEvent: event.BaseEvent{
						EventName:      evt.EventName,
						EventNamespace: clusterPolicy.Namespace,
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
					Compliance:  GetComplianceState(MessageComplianceStateRegex, evt.Message, string(detail.ComplianceState)),
				})
			}
		}
	}
	return events
}

func localReplicatedPolicyEventPostSend(events []interface{}) error {
	for _, policyEvent := range events {
		evt, ok := policyEvent.(*event.ReplicatedPolicyEvent)
		if !ok {
			return fmt.Errorf("failed to type assert to *event.ReplicatedPolicyEvent, event: %v", evt)
		}
		filter.CacheTime(TimeFilterKeyForLocalReplicatedPolicyEvent, evt.CreatedAt)
	}
	return nil
}

func GetComplianceState(regex *regexp.Regexp, message, defaultVal string) string {
	match := regex.FindStringSubmatch(message)
	if len(match) > 1 {
		firstWord := strings.TrimSpace(match[1])
		return firstWord
	}
	return defaultVal
}

func GetRootPolicyAndClusterInfo(ctx context.Context, clusterPolicy *policiesv1.Policy, c client.Client) (
	policy *policiesv1.Policy, clusterID, clusterName string, err error,
) {
	policyNamespacedName := clusterPolicy.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	policy, err = utils.GetRootPolicy(ctx, c, policyNamespacedName)
	if err != nil {
		return nil, "", "", err
	}

	clusterPolicyName := clusterPolicy.Namespace + "." + clusterPolicy.Name

	clusterName, ok := clusterPolicy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return policy, "", "",
			fmt.Errorf("label %s not found in replicated policy %s", constants.PolicyEventClusterNameLabelKey,
				clusterPolicyName)
	}
	clusterID, err = utils.GetClusterId(ctx, c, clusterName)
	if err != nil {
		return policy, "", "", fmt.Errorf("failed to get clusterId by replicated policy %s: %w", clusterPolicyName, err)
	}
	if clusterID == "" {
		return policy, "", "", fmt.Errorf("clusterID not found in replicated policy %s/%s", clusterPolicy.Namespace,
			clusterPolicy.Name)
	}
	return policy, clusterID, clusterName, err
}
