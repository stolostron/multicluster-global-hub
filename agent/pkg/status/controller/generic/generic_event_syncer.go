package generic

import (
	"context"
	"fmt"
	"sync"
	"time"

	cecontext "github.com/cloudevents/sdk-go/v2/context"
	"github.com/go-logr/logr"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/status/controller/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/transport/kafka_confluent"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type genericEventSyncer struct {
	log              logr.Logger
	runtimeClient    client.Client
	producer         transport.Producer
	eventControllers []EventController
	emitter          Emitter
	leafHubName      string
	syncIntervalFunc func() time.Duration
	startOnce        sync.Once

	// share properties with multi-object-controller
	lock *sync.Mutex
}

// LaunchGenericObjectSyncer is used to send multi event(by the eventEmitter) by a specific client.Object
func LaunchGenericEventSyncer(name string, mgr ctrl.Manager, eventControllers []EventController,
	producer transport.Producer, intervalFunc func() time.Duration, emitter Emitter,
) error {
	syncer := &genericEventSyncer{
		log:           ctrl.Log.WithName(name),
		leafHubName:   config.GetLeafHubName(),
		runtimeClient: mgr.GetClient(),

		eventControllers: eventControllers,
		emitter:          emitter,
		producer:         producer,
		syncIntervalFunc: intervalFunc,
		lock:             &sync.Mutex{},
	}

	// start the periodic syncer
	syncer.startOnce.Do(func() {
		go syncer.periodicSync()
	})

	// start all the controllers to update the payload
	for _, eventController := range eventControllers {
		err := AddEventController(mgr, eventController, emitter, syncer.lock)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *genericEventSyncer) periodicSync() {
	currentSyncInterval := s.syncIntervalFunc()
	s.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
	ticker := time.NewTicker(currentSyncInterval)

	for {
		<-ticker.C // wait for next time interval
		s.syncEvent()

		resolvedInterval := s.syncIntervalFunc()

		// reset ticker if sync interval has changed
		if resolvedInterval != currentSyncInterval {
			currentSyncInterval = resolvedInterval
			ticker.Reset(currentSyncInterval)
			s.log.Info(fmt.Sprintf("sync interval has been reset to %s", currentSyncInterval.String()))
		}
	}
}

func (s *genericEventSyncer) syncEvent() {
	s.lock.Lock() // make sure bundles are not updated if we're during bundles sync
	defer s.lock.Unlock()

	if s.emitter.ShouldSend() {
		evt, err := s.emitter.ToCloudEvent()
		if err != nil {
			s.log.Error(err, "failed to get CloudEvent instance", "evt", evt)
		}

		ctx := context.TODO()
		if s.emitter.Topic() != "" {
			ctx = cecontext.WithTopic(ctx, s.emitter.Topic())
		}
		evtCtx := kafka_confluent.WithMessageKey(ctx, s.leafHubName)
		if err := s.producer.SendEvent(evtCtx, *evt); err != nil {
			s.log.Error(err, "failed to send event", "evt", evt)
			return
		}
		s.emitter.PostSend()
	}
}
