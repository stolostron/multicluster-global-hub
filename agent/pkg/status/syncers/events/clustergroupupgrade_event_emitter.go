package events

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/filter"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
)

var TimeFilterKeyForClusterGroupUpgrade = enum.ShortenEventType(string(enum.ClusterGroupUpgradesEventType))

func init() {
	filter.RegisterTimeFilter(TimeFilterKeyForClusterGroupUpgrade)
}

func clusterGroupUpgradePostSend(events []interface{}) error {
	for _, upgradeEvent := range events {
		evt, ok := upgradeEvent.(*models.ClusterGroupUpgradeEvent)
		if !ok {
			return fmt.Errorf("failed to type assert to models.ClusterGroupUpgradeEvent, event: %v", upgradeEvent)
		}
		filter.CacheTime(TimeFilterKeyForClusterGroupUpgrade, evt.CreatedAt)
	}
	return nil
}

// clusterGroupUpgradeEventPredicate filters events for ClusterGroupUpgrade resources
func clusterGroupUpgradeEventPredicate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	if evt.InvolvedObject.Kind != constants.ClusterGroupUpgradeKind {
		return false
	}

	if !filter.Newer(TimeFilterKeyForClusterGroupUpgrade, getEventLastTime(evt).Time) {
		log.Debugw("event filtered:", "event", evt.Namespace+"/"+evt.Name, "eventTime", getEventLastTime(evt).Time)
		return false
	}

	return true
}

// clusterGroupUpgradeEventTransform transforms k8s Event to ClusterGroupUpgradeEvent
func clusterGroupUpgradeEventTransform(runtimeClient client.Client, obj client.Object) interface{} {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		log.Errorw("failed to type assert to corev1.Event", "object", obj.GetName())
		return nil
	}

	// Get the ClusterGroupUpgrade name
	upgradeName := evt.InvolvedObject.Name

	annsJSONB, err := json.Marshal(evt.Annotations)
	if err != nil {
		// Log error but continue processing
		annsJSONB = nil
	}

	return &models.ClusterGroupUpgradeEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		EventAnns:           annsJSONB,
		Message:             evt.Message,
		Reason:              evt.Reason,
		CGUName:             upgradeName,
		LeafHubName:         configs.GetLeafHubName(),
		ReportingController: evt.ReportingController,
		ReportingInstance:   evt.ReportingInstance,
		EventType:           evt.Type,
		CreatedAt:           getEventLastTime(evt).Time,
	}
}
