package syncers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type genericDispatcher struct {
	log         logr.Logger
	consumer    transport.Consumer
	agentConfig config.AgentConfig
	syncers     map[string]Syncer
}

func NewGenericDispatcher(consumer transport.Consumer, config config.AgentConfig) *genericDispatcher {
	return &genericDispatcher{
		log:         ctrl.Log.WithName("spec-bundle-dispatcher"),
		consumer:    consumer,
		agentConfig: config,
		syncers:     make(map[string]Syncer),
	}
}

func (d *genericDispatcher) RegisterSyncer(messageID string, syncer Syncer) {
	d.syncers[messageID] = syncer
	d.log.Info("dispatch syncer is registered", "messageID", messageID)
}

// Start function starts bundles spec syncer.
func (d *genericDispatcher) Start(ctx context.Context) error {
	d.log.Info("started dispatching received bundles...")

	go d.dispatch(ctx)

	<-ctx.Done() // blocking wait for stop event
	d.log.Info("stopped dispatching bundles")

	return nil
}

func (d *genericDispatcher) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-d.consumer.EventChan():
			// if destination is explicitly specified and does not match, drop bundle
			if evt.Source() != transport.Broadcast && evt.Source() != d.agentConfig.LeafHubName {
				continue
			}
			syncer, found := d.syncers[evt.Type()]
			if !found {
				d.log.V(2).Info("dispatching to the default generic syncer", "eventType", evt.Type())
				syncer = d.syncers[GenericMessageKey]
			}
			if syncer == nil || evt == nil {
				d.log.Info("nil syncer or evt", "syncer", syncer, "event", evt)
				continue
			}
			if err := syncer.Sync(evt.Data()); err != nil {
				d.log.Error(err, "submit to syncer error", "eventType", evt.Type())
			}
		}
	}
}
