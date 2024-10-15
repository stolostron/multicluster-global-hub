package managedhub

import (
	cloudevents "github.com/cloudevents/sdk-go/v2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/generic"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/interfaces"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/syncers/configmap"
	genericdata "github.com/stolostron/multicluster-global-hub/pkg/bundle/generic"
	eventversion "github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

func LaunchHubClusterHeartbeatSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	return generic.LaunchMultiObjectSyncer(
		"status.hub_cluster_heartbeat",
		mgr,
		nil,
		producer,
		configmap.GetHeartbeatDuration,
		NewHeartbeatEmitter(),
	)
}

var _ interfaces.Emitter = &heartbeatEmitter{}

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

func (s *heartbeatEmitter) PostUpdate() {
	s.currentVersion.Incr()
}

func (s *heartbeatEmitter) ToCloudEvent(data interface{}) (*cloudevents.Event, error) {
	e := cloudevents.NewEvent()
	e.SetSource(configs.GetLeafHubName())
	e.SetType(string(s.eventType))
	e.SetExtension(eventversion.ExtVersion, s.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, genericdata.GenericObjectBundle{})
	return &e, err
}

func (s *heartbeatEmitter) Topic() string    { return "" }
func (s *heartbeatEmitter) ShouldSend() bool { return true }
func (s *heartbeatEmitter) PostSend(data interface{}) {
	s.currentVersion.Next()
	s.lastSentVersion = *s.currentVersion
}
