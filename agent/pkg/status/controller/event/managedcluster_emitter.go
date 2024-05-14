package event

import (
	"context"
	"fmt"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/filter"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/event"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

var _ generic.ObjectEmitter = &managedClusterEmitter{}

type managedClusterEmitter struct {
	ctx             context.Context
	name            string
	log             logr.Logger
	runtimeClient   client.Client
	eventType       string
	topic           string
	currentVersion  *version.Version
	lastSentVersion version.Version
	payload         event.ManagedClusterEventBundle
}

func NewManagedClusterEventEmitter(ctx context.Context, c client.Client, topic string) *managedClusterEmitter {
	name := strings.Replace(string(enum.ManagedClusterEventType), enum.EventTypePrefix, "", -1)
	filter.RegisterTimeFilter(name)
	return &managedClusterEmitter{
		ctx:             ctx,
		name:            name,
		log:             ctrl.Log.WithName(name),
		eventType:       string(enum.ManagedClusterEventType),
		topic:           topic,
		runtimeClient:   c,
		currentVersion:  version.NewVersion(),
		lastSentVersion: *version.NewVersion(),
		payload:         make([]models.ManagedClusterEvent, 0),
	}
}

func (h *managedClusterEmitter) PostUpdate() {
	h.currentVersion.Incr()
}

func (h *managedClusterEmitter) ShouldUpdate(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	if evt.InvolvedObject.Kind != constants.ManagedClusterKind {
		return false
	}

	// if it's a older event, then return false
	if !filter.Newer(h.name, evt.CreationTimestamp.Time) {
		return false
	}

	return true
}

func (h *managedClusterEmitter) Update(obj client.Object) bool {
	evt, ok := obj.(*corev1.Event)
	if !ok {
		return false
	}

	cluster, err := getInvolveCluster(h.ctx, h.runtimeClient, evt)
	if err != nil {
		h.log.Error(err, "failed to get involved cluster", "event", evt.Namespace+"/"+evt.Name, "cluster", cluster.Name)
		return false
	}

	clusterId, err := utils.GetClusterId(h.ctx, h.runtimeClient, cluster.Name)
	if err != nil {
		h.log.Error(err, "failed to get involved clusterId", "event", evt.Namespace+"/"+evt.Name)
		return false
	}

	// update
	clusterEvent := models.ManagedClusterEvent{
		EventName:           evt.Name,
		EventNamespace:      evt.Namespace,
		Message:             evt.Message,
		Reason:              evt.Reason,
		ClusterID:           clusterId,
		LeafHubName:         config.GetLeafHubName(),
		ReportingController: evt.ReportingController,
		ReportingInstance:   evt.ReportingInstance,
		EventType:           evt.Type,
		CreatedAt:           evt.CreationTimestamp.Time,
	}
	h.payload = append(h.payload, clusterEvent)
	return true
}

func (*managedClusterEmitter) Delete(client.Object) bool {
	// do nothing
	return false
}

func (h *managedClusterEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	if len(h.payload) < 1 {
		return nil, fmt.Errorf("the cloudevent instance shouldn't be nil")
	}
	e := cloudevents.NewEvent()
	e.SetType(h.eventType)
	e.SetSource(config.GetLeafHubName())
	e.SetExtension(version.ExtVersion, h.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, h.payload)
	return &e, err
}

// to assert whether emit the current cloudevent
func (h *managedClusterEmitter) ShouldSend() bool {
	return h.currentVersion.NewerThan(&h.lastSentVersion)
}

func (h *managedClusterEmitter) Topic() string {
	return h.topic
}

func (h *managedClusterEmitter) PostSend() {
	// update the time filter: with latest event
	for _, evt := range h.payload {
		filter.CacheTime(h.name, evt.CreatedAt)
	}
	// update version and clean the cache
	h.payload = make([]models.ManagedClusterEvent, 0)
	// 1. the version get into the next generation
	// 2. set the lastSenteVersion to current version
	h.currentVersion.Next()
	h.lastSentVersion = *h.currentVersion
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
