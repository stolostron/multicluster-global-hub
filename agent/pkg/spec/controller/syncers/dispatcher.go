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
		case message := <-d.consumer.MessageChan():
			// if destination is explicitly specified and does not match, drop bundle
			if message.Destination != transport.Broadcast &&
				message.Destination != d.agentConfig.LeafHubName {
				continue
			}
			syncer, found := d.syncers[message.ID]
			if !found {
				d.log.Info("dispatching to the default generic syncer", "messageID", message.ID)
				syncer = d.syncers[GenericMessageKey]
			}
			if err := syncer.Sync(message.Payload); err != nil {
				d.log.Error(err, "submit to syncer error", "messageID", message.ID)
			}
		}
	}
}
