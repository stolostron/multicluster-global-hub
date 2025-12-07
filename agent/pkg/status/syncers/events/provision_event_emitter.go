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
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var TimeFilterKeyForProvision = enum.ShortenEventType(string(enum.ProvisionEventType))

func provisionPostSend(events []interface{}) error {
	for _, provisionEvent := range events {
		evt, ok := provisionEvent.(*models.ManagedClusterEvent)
		if !ok {
			return fmt.Errorf("failed to type assert to models.ManagedClusterEvent, event: %v", provisionEvent)
		}
		// Cache event timestamp to prevent duplicate collection
		filter.CacheTime(TimeFilterKeyForProvision, evt.CreatedAt)
	}
	return nil
}

// provisionEventPredicate filters events for provision job events
// First-layer filter: InvolvedObject.Kind == "Job"
// Second-layer filter: Job name ends with "-provision" AND starts with namespace
// Third-layer filter: Time filter to prevent duplicates
func provisionEventPredicate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		log.Errorw("failed to type assert to corev1.Event", "object", obj.GetName())
		return false
	}

	// First-layer filter: Must be a Job event
	if evt.InvolvedObject.Kind != "Job" {
		return false
	}

	// Second-layer filter: Job name must end with "-provision"
	jobName := evt.InvolvedObject.Name
	if !strings.HasSuffix(jobName, "-provision") {
		return false
	}

	// Second-layer filter: Job name must start with the namespace (validates namespace == cluster name)
	// This prevents collecting events from orphaned provision jobs
	// Example: namespace "cluster2" with job "cluster2-0-bvpxh-provision" should match
	//          namespace "cluster2" with job "other-abc-provision" should NOT match
	if !strings.HasPrefix(jobName, evt.Namespace+"-") {
		log.Debugw("event filtered: job name does not start with namespace",
			"event", evt.Namespace+"/"+evt.Name,
			"jobName", jobName,
			"namespace", evt.Namespace)
		return false
	}

	// Third-layer filter: Time filter to prevent duplicate events
	if !filter.Newer(TimeFilterKeyForProvision, getEventLastTime(evt).Time) {
		log.Debugw("event filtered: duplicate event",
			"event", evt.Namespace+"/"+evt.Name,
			"eventTime", getEventLastTime(evt).Time)
		return false
	}

	return true
}

// provisionEventTransform transforms k8s Event to ManagedClusterEvent for provision jobs
// The cluster name is extracted from the event namespace (following OCM convention: namespace == cluster name)
func provisionEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	// Extract cluster name from namespace (OCM convention: namespace == cluster name)
	clusterName := evt.Namespace

	// Validate that the ManagedCluster exists
	cluster, err := getProvisionCluster(context.Background(), runtimeClient, clusterName)
	if err != nil {
		log.Debugw("event filtered: no matching ManagedCluster",
			"clusterName", clusterName,
			"event", evt.Namespace+"/"+evt.Name,
			"error", err)
		return nil
	}

	// Get cluster ID (required for database storage)
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

// getProvisionCluster gets the ManagedCluster by cluster name (from namespace)
func getProvisionCluster(ctx context.Context, c client.Client, clusterName string) (*clusterv1.ManagedCluster, error) {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}
	err := c.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)
	return cluster, err
}
