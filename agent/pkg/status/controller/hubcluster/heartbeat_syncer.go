package hubcluster

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	genericdata "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func LaunchHubClusterHeartbeatSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	return generic.LaunchGenericEventSyncer(
		"status.hub_cluster_heartbeat",
		mgr,
		nil,
		producer,
		config.GetHeartbeatDuration,
		NewHeartbeatEmitter(),
	)
}

var _ generic.Emitter = &heartbeatEmitter{}

func NewHeartbeatEmitter() *heartbeatEmitter {
	emitter := &heartbeatEmitter{
		eventType:       enum.HubClusterHeartbeatType,
		currentVersion:  eventversion.NewVersion(),
		lastSentVersion: *eventversion.NewVersion(),
	}
	return emitter
}

type heartbeatEmitter struct {
	eventType       enum.EventType
	currentVersion  *eventversion.Version
	lastSentVersion eventversion.Version
}

// assert whether to update the payload by the current handler
func (s *heartbeatEmitter) ShouldUpdate(object client.Object) bool { return true }

func (s *heartbeatEmitter) PostUpdate() {
	s.currentVersion.Incr()
}

func (s *heartbeatEmitter) ToCloudEvent() (*cloudevents.Event, error) {
	e := cloudevents.NewEvent()
	e.SetSource(config.GetLeafHubName())
	e.SetType(string(s.eventType))
	e.SetExtension(eventversion.ExtVersion, s.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, genericdata.GenericObjectData{})
	return &e, err
}

func (s *heartbeatEmitter) Topic() string    { return "" }
func (s *heartbeatEmitter) ShouldSend() bool { return true }
func (s *heartbeatEmitter) PostSend() {
	s.currentVersion.Next()
	s.lastSentVersion = *s.currentVersion
}
