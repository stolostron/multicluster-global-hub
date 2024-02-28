package hubcluster

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/generic"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/cluster"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/metadata"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type heartbeatStatusSyncer struct {
	log logr.Logger

	transportBundleKey    string
	lastSentBundleVersion metadata.BundleVersion // not pointer so it does not point to the bundle's internal version
	heartbeatBundle       bundle.BaseAgentBundle

	transport    transport.Producer
	intervalFunc config.ResolveSyncIntervalFunc
	lock         sync.Mutex
}

func AddHeartbeatStatusSyncer(mgr ctrl.Manager, producer transport.Producer) error {
	leafHubName := config.GetLeafHubName()
	clusterHeartbeatBundle := cluster.NewAgentHubClusterHeartbeatBundle(leafHubName)

	statusSyncCtrl := &heartbeatStatusSyncer{
		log: ctrl.Log.WithName("heartbeat-syncer"),

		transportBundleKey:    fmt.Sprintf("%s.%s", leafHubName, constants.HubClusterHeartbeatMsgKey),
		heartbeatBundle:       clusterHeartbeatBundle,
		lastSentBundleVersion: *clusterHeartbeatBundle.BundleVersion,

		transport:    producer,
		intervalFunc: config.GetHeartbeatDuration,
		lock:         sync.Mutex{},
	}
	return mgr.Add(statusSyncCtrl)
}

func (s *heartbeatStatusSyncer) Start(ctx context.Context) error {
	go s.periodicSync(ctx)
	return nil
}

func (s *heartbeatStatusSyncer) periodicSync(ctx context.Context) {
	currentSyncInterval := s.intervalFunc()
	s.log.Info("sync interval has been set to", "interval", currentSyncInterval.String())

	ticker := time.NewTicker(currentSyncInterval)

	for {
		select {
		case <-ctx.Done():
			s.log.Info("ctx is done, and exiting the heartbeat loop!")
			ticker.Stop()
			return
		case <-ticker.C: // wait for next time interval
			s.heartbeatBundle.GetVersion().Incr()
			s.syncBundle(ctx)
			resolvedInterval := s.intervalFunc()
			// reset ticker if sync interval has changed
			if resolvedInterval != currentSyncInterval {
				currentSyncInterval = resolvedInterval
				ticker.Reset(currentSyncInterval)
				s.log.Info("sync interval has been reset to", "interval", currentSyncInterval.String())
			}
		}
	}
}

func (s *heartbeatStatusSyncer) syncBundle(ctx context.Context) {
	s.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer s.lock.Unlock()

	bundleVersion := s.heartbeatBundle.GetVersion()

	// send to transport only if bundle has changed.
	if bundleVersion.NewerThan(&s.lastSentBundleVersion) {

		payloadBytes, err := json.Marshal(s.heartbeatBundle)
		if err != nil {
			s.log.Error(err, "marshal entry.bundle error", "entry.bundleKey", s.transportBundleKey)
			return
		}

		if err := s.transport.Send(ctx, &transport.Message{
			Key:         s.transportBundleKey,
			Destination: config.GetLeafHubName(),
			MsgType:     constants.StatusBundle,
			Payload:     payloadBytes,
		}); err != nil {
			s.log.Error(err, "send transport message error", "key", s.transportBundleKey)
			return
		}

		// 1. get into the next generation
		// 2. set the lastSentBundleVersion to first version of next generation
		s.heartbeatBundle.GetVersion().Next()
		s.lastSentBundleVersion = *s.heartbeatBundle.GetVersion()
	}
}

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
		currentVersion:  metadata.NewBundleVersion(),
		lastSentVersion: *metadata.NewBundleVersion(),
	}
	return emitter
}

type heartbeatEmitter struct {
	eventType       enum.EventType
	currentVersion  *metadata.BundleVersion
	lastSentVersion metadata.BundleVersion
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
	e.SetExtension(metadata.ExtVersion, s.currentVersion.String())
	err := e.SetData(cloudevents.ApplicationJSON, nil)
	return &e, err
}

func (s *heartbeatEmitter) Topic() string    { return "" }
func (s *heartbeatEmitter) ShouldSend() bool { return true }
func (s *heartbeatEmitter) PostSend() {
	s.currentVersion.Next()
	s.lastSentVersion = *s.currentVersion
}
