package handlers

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

func NewManagedClusterEventEmitter() interfaces.Emitter {
	name := strings.ReplaceAll(string(enum.ManagedClusterEventType), enum.EventTypePrefix, "")
	return generic.NewGenericEmitter(enum.ManagedClusterEventType, generic.WithPostSend(
		// After sending the event, update the filter cache and clear the bundle from the handler cache.
		func(data interface{}) {
			events, ok := data.(*event.ManagedClusterEventBundle)
			if !ok {
				return
			}
			// update the time filter: with latest event
			for _, evt := range *events {
				filter.CacheTime(name, evt.CreatedAt)
			}
			// reset the payload
			*events = (*events)[:0]
		}),
	)
}

type managedClusterEventHandler struct {
	ctx           context.Context
	name          string
	runtimeClient client.Client
	eventType     string
	payload       *event.ManagedClusterEventBundle
}

func NewManagedClusterEventHandler(ctx context.Context, c client.Client) *managedClusterEventHandler {
	name := strings.ReplaceAll(string(enum.ManagedClusterEventType), enum.EventTypePrefix, "")
	filter.RegisterTimeFilter(name)
	return &managedClusterEventHandler{
		ctx:           ctx,
		name:          name,
		eventType:     string(enum.ManagedClusterEventType),
		runtimeClient: c,
		payload:       &event.ManagedClusterEventBundle{},
	}
}

func (h *managedClusterEventHandler) Get() interface{} {
	return h.payload
}

func (h *managedClusterEventHandler) shouldUpdate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	if evt.InvolvedObject.Kind != constants.ManagedClusterKind {
		return false
	}

	// if it's a older event, then return false
	if !filter.Newer(h.name, getEventLastTime(evt).Time) {
		return false
	}

	return true
}

func (h *managedClusterEventHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	cluster, err := getInvolveCluster(h.ctx, h.runtimeClient, evt)
	if err != nil {

		log.Errorf("failed to get cluster: %s, event: %s/%s, error: %v", cluster.Name, evt.Namespace, evt.Name, err)
		return false
	}

	clusterId, err := utils.GetClusterId(h.ctx, h.runtimeClient, cluster.Name)
	if err != nil {
		log.Errorf("failed to get clusterId: %s, event: %s/%s, error: %v", cluster.Name, evt.Namespace, evt.Name, err)
		return false
	}

	clusterEvent := models.ManagedClusterEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		Message:             evt.Message,
		Reason:              evt.Reason,
		ClusterName:         cluster.Name,
		ClusterID:           clusterId,
		LeafHubName:         configs.GetLeafHubName(),
		ReportingController: evt.ReportingController,
		ReportingInstance:   evt.ReportingInstance,
		EventType:           evt.Type,
		CreatedAt:           getEventLastTime(evt).Time,
	}

	*h.payload = append(*h.payload, clusterEvent)
	return true
}

func (*managedClusterEventHandler) Delete(client.Object) bool {
	// do nothing
	return false
}

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
