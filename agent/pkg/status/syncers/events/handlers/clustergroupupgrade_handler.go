package handlers

import (
	"context"
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

func NewClusterGroupUpgradeEventEmitter() interfaces.Emitter {
	name := strings.Replace(string(enum.CGUEventType), enum.EventTypePrefix, "", -1)
	return generic.NewGenericEmitter(enum.CGUEventType, generic.WithPostSend(
		// After sending the event, update the filter cache and clear the bundle from the handler cache.
		func(data interface{}) {
			events, ok := data.(*event.ClusterGroupUpgradeEventBundle)
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

type clusterGroupUpgradeEventHandler struct {
	ctx           context.Context
	name          string
	runtimeClient client.Client
	eventType     string
	payload       *event.ClusterGroupUpgradeEventBundle
}

func NewClusterGroupUpgradeEventHandler(ctx context.Context, c client.Client) *clusterGroupUpgradeEventHandler {
	name := strings.Replace(string(enum.CGUEventType), enum.EventTypePrefix, "", -1)
	filter.RegisterTimeFilter(name)
	return &clusterGroupUpgradeEventHandler{
		ctx:           ctx,
		name:          name,
		eventType:     string(enum.CGUEventType),
		runtimeClient: c,
		payload:       &event.ClusterGroupUpgradeEventBundle{},
	}
}

func (h *clusterGroupUpgradeEventHandler) shouldUpdate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	if evt.InvolvedObject.Kind != constants.ClusterGroupUpgradeKind {
		return false
	}

	// if it's a older event, then return false
	if !filter.Newer(h.name, getEventLastTime(evt).Time) {
		return false
	}

	return true
}

func (h *clusterGroupUpgradeEventHandler) Update(obj client.Object) bool {
	if !h.shouldUpdate(obj) {
		return false
	}

	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	annsJSONB, err := json.Marshal(evt.Annotations)
	if err != nil {
		log.Error("failed to parse the event annotations %+v: %s", evt.Annotations, err.Error())
	}

	clusterEvent := models.ClusterGroupUpgradeEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		EventAnns:           annsJSONB,
		Message:             evt.Message,
		Reason:              evt.Reason,
		CGUName:             evt.InvolvedObject.Name,
		LeafHubName:         configs.GetLeafHubName(),
		ReportingController: evt.ReportingController,
		ReportingInstance:   evt.ReportingInstance,
		EventType:           evt.Type,
		CreatedAt:           getEventLastTime(evt).Time,
	}

	*h.payload = append(*h.payload, clusterEvent)
	return true
}

func (*clusterGroupUpgradeEventHandler) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *clusterGroupUpgradeEventHandler) Get() interface{} {
	return h.payload
}
