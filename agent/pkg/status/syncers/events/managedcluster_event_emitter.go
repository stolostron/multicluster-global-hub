package events

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/emitters"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var TimeFilterKeyForManagedCluster = enum.ShortenEventType(string(enum.ManagedClusterEventType))

// NewManagedClusterEventEmitter creates an EventEmitter for ManagedCluster events
func NewManagedClusterEventEmitter(producer transport.Producer, runtimeClient client.Client,
	eventMode constants.EventSendMode,
) *emitters.EventEmitter {
	return emitters.NewEventEmitter(
		enum.ManagedClusterEventType,
		producer,
		runtimeClient,
		managedClusterEventPredicate,
		managedClusterEventTransform,
		eventMode,
		emitters.WithPostSend(
			func(events []interface{}) error {
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
			}),
	)
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
func managedClusterEventPredicate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		log.Errorw("failed to type assert to corev1.Event", "object", obj.GetName())
		return false
	}

	if evt.InvolvedObject.Kind != constants.ManagedClusterKind {
		log.Debugw("event filtered: not a ManagedCluster event", "event", evt.Namespace+"/"+evt.Name)
		return false
	}

	if !filter.Newer(TimeFilterKeyForManagedCluster, getEventLastTime(evt).Time) {
		log.Debugw("event filtered:", "event", evt.Namespace+"/"+evt.Name, "eventTime", getEventLastTime(evt).Time)
		return false
	}
	return true
}

// managedClusterEventTransform transforms k8s Event to ManagedClusterEvent
func managedClusterEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return nil
	}

	clusterName := evt.InvolvedObject.Name

	cluster, err := getInvolveCluster(context.Background(), runtimeClient, evt)
	if err != nil {
		log.Errorw("failed to get cluster", "cluster", clusterName, "event", evt.Namespace+"/"+evt.Name, "error", err)
		return nil
	}

	clusterId := utils.GetClusterClaimID(cluster)
	if clusterId == "" {
		log.Errorw("failed to get clusterId", "cluster", clusterName, "event", evt.Namespace+"/"+evt.Name,
			"cluster", cluster)
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

// getInvolveCluster gets the ManagedCluster involved in the event
func getInvolveCluster(ctx context.Context, c client.Client, evt *corev1.Event) (*clusterv1.ManagedCluster, error) {
	cluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      evt.InvolvedObject.Name,
			Namespace: evt.InvolvedObject.Namespace,
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
