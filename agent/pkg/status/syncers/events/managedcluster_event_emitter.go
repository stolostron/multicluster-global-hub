package events

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var TimeFilterKeyForManagedCluster = enum.ShortenEventType(string(enum.ManagedClusterEventType))

// isValidProvisionJob validates if a job name matches the provision job pattern
// Pattern: <namespace>-<hash>-provision
// Returns true if valid, false otherwise.
func isValidProvisionJob(jobName, namespace string) bool {
	// Must end with "-provision" suffix
	const suffix = "-provision"
	if !strings.HasSuffix(jobName, suffix) {
		return false
	}

	// Must start with "namespace-"
	prefix := namespace + "-"
	if !strings.HasPrefix(jobName, prefix) {
		return false
	}

	// Extract middle part (hash) between namespace and "-provision"
	// Example: "cluster2-0-bvpxh-provision" -> middle = "0-bvpxh"
	// The middle part must: start with '-' and end with '-'
	expectedLength := len(prefix) + len(suffix)
	if len(jobName) <= expectedLength {
		// No room for hash between prefix and suffix
		return false
	}

	// Verify there's actual content between prefix and suffix
	middle := jobName[len(prefix) : len(jobName)-len(suffix)]
	return len(middle) > 0
}

func managedClusterPostSend(events []interface{}) error {
	for _, clusterEvent := range events {
		evt, ok := clusterEvent.(*models.ManagedClusterEvent)
		if !ok {
			return fmt.Errorf("failed to type assert to models.ManagedClusterEvent, event: %v", clusterEvent)
		}
		// To avoid duplicate events, apply a time filter in the predicate and prevent sending the same event
		// multiple times.
		filter.CacheTime(TimeFilterKeyForManagedCluster, evt.CreatedAt)
	}
	return nil
}

// managedClusterEventPredicate filters events for ManagedCluster resources
// Handles two types of events:
// 1. Direct ManagedCluster events (InvolvedObject.Kind == "ManagedCluster")
// 2. Provision Job events (InvolvedObject.Kind == "Job" with "-provision" suffix)
func managedClusterEventPredicate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		log.Errorw("failed to type assert to corev1.Event", "object", obj.GetName())
		return false
	}

	// Use tagged switch to handle different event kinds (fixes staticcheck QF1003)
	switch evt.InvolvedObject.Kind {
	case constants.ManagedClusterKind:
		// Direct ManagedCluster events - always accept (time filter applied later)
		// No additional validation needed

	case "Job":
		// Provision Job events - validate job name pattern
		jobName := evt.InvolvedObject.Name
		if !isValidProvisionJob(jobName, evt.Namespace) {
			log.Debugw("event filtered: invalid provision job name pattern",
				"event", evt.Namespace+"/"+evt.Name,
				"jobName", jobName,
				"namespace", evt.Namespace)
			return false
		}

	default:
		// Not a ManagedCluster or provision Job event
		return false
	}

	// Unified time filter for all accepted events (applied once at the end)
	if !filter.Newer(TimeFilterKeyForManagedCluster, getEventLastTime(evt).Time) {
		log.Debugw("event filtered: duplicate event",
			"event", evt.Namespace+"/"+evt.Name,
			"kind", evt.InvolvedObject.Kind,
			"eventTime", getEventLastTime(evt).Time)
		return false
	}

	return true
}

// managedClusterEventTransform transforms k8s Event to ManagedClusterEvent
// Handles two types of events:
// 1. Direct ManagedCluster events: clusterName = InvolvedObject.Name
// 2. Provision Job events: clusterName = Namespace (OCM convention)
func managedClusterEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	var clusterName string

	// Determine cluster name based on event type (use tagged switch for consistency)
	switch evt.InvolvedObject.Kind {
	case constants.ManagedClusterKind:
		// Direct ManagedCluster event
		clusterName = evt.InvolvedObject.Name
	case "Job":
		// Provision Job event - cluster name is the namespace
		clusterName = evt.Namespace
	default:
		// Should not happen due to predicate filtering, but handle gracefully
		log.Errorw("unexpected event kind", "kind", evt.InvolvedObject.Kind, "event", evt.Namespace+"/"+evt.Name)
		return nil
	}

	cluster, err := getInvolveCluster(context.Background(), runtimeClient, clusterName)
	if err != nil {
		log.Debugw("event filtered: no matching ManagedCluster",
			"clusterName", clusterName,
			"event", evt.Namespace+"/"+evt.Name,
			"error", err)
		return nil
	}

	clusterId := utils.GetClusterClaimID(cluster, string(cluster.GetUID()))
	if clusterId == "" {
		log.Errorw("failed to get clusterId",
			"cluster", clusterName,
			"event", evt.Namespace+"/"+evt.Name)
		return nil
	}

	return &models.ManagedClusterEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		Message:             evt.Message,
		Reason:              evt.Reason,
		ClusterName:         clusterName,
		ClusterID:           clusterId,
		LeafHubName:         configs.GetLeafHubName(),
		ReportingController: evt.ReportingController,
		ReportingInstance:   evt.ReportingInstance,
		EventType:           evt.Type,
		CreatedAt:           getEventLastTime(evt).Time,
	}
}

// getInvolveCluster gets the ManagedCluster by cluster name
func getInvolveCluster(ctx context.Context, c client.Client, clusterName string) (*clusterv1.ManagedCluster, error) {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	return cluster, err
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
