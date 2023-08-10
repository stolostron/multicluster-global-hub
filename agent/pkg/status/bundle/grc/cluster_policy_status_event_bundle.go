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
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

// NewClustersPerPolicyBundle creates a new instance of ClustersPerPolicyBundle.
func NewClusterPolicyHistoryEventBundle(ctx context.Context, leafHubName string, incarnation uint64,
	runtimeClient client.Client,
) bundlepkg.Bundle {
	return &ClusterPolicyHistoryEventBundle{
		BaseClusterPolicyStatusEventBundle: statusbundle.BaseClusterPolicyStatusEventBundle{
			PolicyStatusEvents: make(map[string][]*models.LocalClusterPolicyEvent),
			LeafHubName:        leafHubName,
			BundleVersion:      statusbundle.NewBundleVersion(incarnation, 0),
		},
		lock:          sync.Mutex{},
		runtimeClient: runtimeClient,
		ctx:           context.Background(),
		regex:         regexp.MustCompile(`(\w+);`),
		log:           ctrl.Log.WithName("cluster-policy-status-event-bundle"),
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
	if policy.Status.Details == nil {
		return // no status to update
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

	// update the object to bundle
	bundlePolicyStatusEvents, ok := bundle.PolicyStatusEvents[string(policy.GetUID())]
	if !ok {
		bundlePolicyStatusEvents = make([]*models.LocalClusterPolicyEvent, 0)
	}

	// deprecated events, cause it has been synced before
	deprecatedBundleEvents := make(map[string]*models.LocalClusterPolicyEvent)
	for _, e := range bundlePolicyStatusEvents {
		deprecatedBundleEvents[e.EventName] = e
	}

	for _, detail := range policy.Status.Details {
		if detail.History != nil {
			for _, event := range detail.History {
				bundlePolicyStatusEvents = bundle.updatePolicyEvents(event,
					string(detail.ComplianceState), deprecatedBundleEvents,
					string(rootPolicy.GetUID()), clusterId, bundlePolicyStatusEvents)
				delete(deprecatedBundleEvents, event.EventName)
			}
		}
	}

	// only load the 'new' events to bundle
	deltaPolicyEvents := make([]*models.LocalClusterPolicyEvent, 0)
	for _, event := range bundlePolicyStatusEvents {
		if _, ok := deprecatedBundleEvents[event.EventName]; !ok {
			deltaPolicyEvents = append(deltaPolicyEvents, event)
		}
	}

	if len(deltaPolicyEvents) > 0 {
		bundle.PolicyStatusEvents[string(policy.GetUID())] = deltaPolicyEvents
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

// add/update the current status events to bundle, remove the updated event from deprecatedBundleEvents
func (bundle *ClusterPolicyHistoryEventBundle) updatePolicyEvents(event policiesv1.ComplianceHistory,
	parentCompliance string, deprecatedBundleEvents map[string]*models.LocalClusterPolicyEvent,
	rootPolicyId, clusterId string, bundlePolicyStatusEvents []*models.LocalClusterPolicyEvent,
) []*models.LocalClusterPolicyEvent {
	compliance := bundle.ParseCompliance(event.Message)
	if compliance == "" {
		compliance = parentCompliance
	}
	eventTime := event.LastTimestamp.Time
	bundleEvent, ok := deprecatedBundleEvents[event.EventName]
	if ok {
		if !bundleEvent.CreatedAt.Equal(eventTime) {
			bundleEvent.Message = event.Message
			bundleEvent.Count++
			bundleEvent.CreatedAt = eventTime
			bundleEvent.Compliance = compliance
			bundlePolicyStatusEvents = append(bundlePolicyStatusEvents, bundleEvent)

			// the event is updated, remove it from deprecatedBundleEvents
			delete(deprecatedBundleEvents, event.EventName)
		}
	} else {
		bundlePolicyStatusEvents = append(bundlePolicyStatusEvents,
			&models.LocalClusterPolicyEvent{
				BaseLocalPolicyEvent: models.BaseLocalPolicyEvent{
					EventName:  event.EventName,
					PolicyID:   rootPolicyId,
					Message:    event.Message,
					Reason:     "PolicyStatusSync", // using this value as a placeholder
					Source:     nil,
					Count:      1,
					Compliance: compliance,
					CreatedAt:  eventTime,
				},
				ClusterID: clusterId,
			})
	}
	return bundlePolicyStatusEvents
}
