package grc

import (
	"context"
	"regexp"
	"strings"
	"sync"

	"github.com/go-logr/logr"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/helper"
	bundlepkg "github.com/stolostron/multicluster-global-hub/agent/pkg/status/bundle"
	statusbundle "github.com/stolostron/multicluster-global-hub/pkg/bundle/status"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
)

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClusterPolicyHistoryEventBundle(ctx context.Context, leafHubName string, incarnation uint64,
	runtimeClient client.Client,
) bundlepkg.Bundle {
	return &ClusterPolicyHistoryEventBundle{
		BaseClusterPolicyStatusEventBundle: statusbundle.BaseClusterPolicyStatusEventBundle{
			PolicyStatusEvents: make(map[string][]*statusbundle.PolicyStatusEvent),
			LeafHubName:        leafHubName,
			BundleVersion:      statusbundle.NewBundleVersion(incarnation, 0),
		},
		lock:          sync.Mutex{},
		runtimeClient: runtimeClient,
		ctx:           context.Background(),
		regex:         regexp.MustCompile(`(\w+);`),
		log:           ctrl.Log.WithName("cluster-policy-history-event-bundle"),
	}
}

type ClusterPolicyHistoryEventBundle struct {
	statusbundle.BaseClusterPolicyStatusEventBundle
	lock          sync.Mutex
	runtimeClient client.Client
	ctx           context.Context
	regex         *regexp.Regexp
	log           logr.Logger
}

// UpdateObject function to update a single object inside a bundle.
func (bundle *ClusterPolicyHistoryEventBundle) UpdateObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, ok := object.(*policiesv1.Policy)
	if !ok {
		return // do not handle objects other than policy
	}
	bundlePolicyStatusEvents, ok := bundle.PolicyStatusEvents[string(policy.GetUID())]
	if !ok {
		bundlePolicyStatusEvents = make([]*statusbundle.PolicyStatusEvent, 0)
	}

	if policy.Status.Details == nil {
		return // no status to update
	}

	eventMap := make(map[string]*statusbundle.PolicyStatusEvent)
	for _, e := range bundlePolicyStatusEvents {
		eventMap[e.EventName] = e
	}

	// root policy id
	rootPolicyNamespacedName, ok := policy.Labels[constants.PolicyEventRootPolicyNameLabelKey]
	if !ok {
		return
	}
	rootPolicy, err := helper.GetRootPolicy(bundle.ctx, bundle.runtimeClient, rootPolicyNamespacedName)
	if err != nil {
		return
	}

	// cluster id
	clusterName, ok := policy.Labels[constants.PolicyEventClusterNameLabelKey]
	if !ok {
		return
	}
	clusterId, err := helper.GetClusterId(bundle.ctx, bundle.runtimeClient, clusterName)
	if err != nil {
		return
	}

	modified := false
	for _, detail := range policy.Status.Details {
		if detail.History != nil {
			for _, event := range detail.History {
				bundle.loadEventToBundle(event, detail, eventMap, string(rootPolicy.GetUID()), clusterId,
					bundlePolicyStatusEvents, &modified)
			}
		}
	}

	if modified {
		bundle.BundleVersion.Generation++
	}
}

// DeleteObject function to delete a single object inside a bundle.
func (bundle *ClusterPolicyHistoryEventBundle) DeleteObject(object bundlepkg.Object) {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	policy, isPolicy := object.(*policiesv1.Policy)
	if !isPolicy {
		return // do not handle objects other than policy
	}

	delete(bundle.PolicyStatusEvents, string(policy.GetUID()))
	// bundle.BundleVersion.Generation++ // if the policy is deleted, we don't need to delete the event from database
}

// GetBundleVersion function to get bundle version.
func (bundle *ClusterPolicyHistoryEventBundle) GetBundleVersion() *statusbundle.BundleVersion {
	bundle.lock.Lock()
	defer bundle.lock.Unlock()

	return bundle.BundleVersion
}

func (bundle *ClusterPolicyHistoryEventBundle) ParseCompliance(message string) string {
	match := bundle.regex.FindStringSubmatch(message)
	if len(match) > 1 {
		firstWord := strings.TrimSpace(match[1])
		return firstWord
	}
	return ""
}

func (bundle *ClusterPolicyHistoryEventBundle) loadEventToBundle(event policiesv1.ComplianceHistory,
	detail *policiesv1.DetailsPerTemplate, eventMap map[string]*statusbundle.PolicyStatusEvent,
	rootPolicyId, clusterId string, bundlePolicyStatusEvents []*statusbundle.PolicyStatusEvent,
	modified *bool,
) []*statusbundle.PolicyStatusEvent {
	compliance := bundle.ParseCompliance(event.Message)
	if compliance == "" {
		compliance = string(detail.ComplianceState)
	}
	bundleEvent, ok := eventMap[event.EventName]
	if ok {
		if bundleEvent.LastTimestamp != event.LastTimestamp {
			bundleEvent.Message = event.Message
			bundleEvent.Count = bundleEvent.Count + 1
			bundleEvent.LastTimestamp = event.LastTimestamp
			bundleEvent.Compliance = compliance
			*modified = true
		}
	} else {
		*modified = true
		bundlePolicyStatusEvents = append(bundlePolicyStatusEvents,
			&statusbundle.PolicyStatusEvent{
				EventName:     event.EventName,
				PolicyID:      rootPolicyId,
				ClusterID:     clusterId,
				Compliance:    compliance,
				LastTimestamp: event.LastTimestamp,
				Message:       event.Message,
				Count:         1,
			})
	}
	return bundlePolicyStatusEvents
}
