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

// isValidProvisionJobEvent validates if a job name matches a cluster lifecycle job pattern
// Supported provision job patterns:
// - ACM console: <namespace>-<hash>-provision
// - ClusterInstance: <namespace>-imageset
// Returns true if valid, false otherwise.
func isValidProvisionJobEvent(evt *corev1.Event) bool {
	if evt.InvolvedObject.Kind != "Job" {
		return false
	}

	namespace := evt.Namespace
	jobName := evt.InvolvedObject.Name

	// Check for imageset job pattern: <namespace>-imageset, e.g. cluster2-imageset
	if jobName == namespace+"-imageset" {
		return true
	}

	// Check for provision job pattern: <namespace>-<hash>-provision, e.g. cluster2-0-bvpxh-provision
	if strings.HasSuffix(jobName, "-provision") && strings.HasPrefix(jobName, namespace+"-") {
		return true
	}

	return false
}

// customizeProvisionJobMessage customizes the message for provision job events
// to provide better user experience and clarity
func customizeProvisionJobMessage(reason, originalMessage, clusterName string) string {
	switch reason {
	case "SuccessfulCreate":
		return fmt.Sprintf("Cluster %s provisioning has started", clusterName)
	case "Completed":
		return fmt.Sprintf("Cluster %s provisioning completed successfully", clusterName)
	default:
		return fmt.Sprintf("Cluster %s provisioning: %s", clusterName, originalMessage)
	}
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

	// Unified time filter for all accepted events (applied once at the end)
	if !filter.Newer(TimeFilterKeyForManagedCluster, getEventLastTime(evt).Time) {
		log.Debugw("event filtered: expired event", "event", evt.Name, "kind", evt.InvolvedObject.Kind,
			"time", getEventLastTime(evt).Time)
		return false
	}

	if evt.InvolvedObject.Kind == constants.ManagedClusterKind {
		return true
	}

	if isValidProvisionJobEvent(evt) {
		return true
	}

	return false
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
		log.Debugw("event filtered: no matching cluster", "clusterName", clusterName, "event", evt.Name, "error", err)
		return nil
	}

	clusterId := utils.GetClusterClaimID(cluster, string(cluster.GetUID()))
	if clusterId == "" {
		log.Warnw("failed to get clusterId", "cluster", clusterName, "event", evt.Name)
		return nil
	}

	// Customize message for provision job events
	message := evt.Message
	if isValidProvisionJobEvent(evt) {
		message = customizeProvisionJobMessage(evt.Reason, evt.Message, clusterName)
	}

	return &models.ManagedClusterEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		Message:             message,
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
