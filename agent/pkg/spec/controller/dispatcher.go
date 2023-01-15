package controller

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type Dispatcher interface {
	Start(ctx context.Context) error
	// the channel delivers payload message to specific syncer
	RegisterChannel(messageID string, channel chan interface{})
}

type genericDispatcher struct {
	log         logr.Logger
	consumer    transport.Consumer
	workerPool  *workers.WorkerPool
	agentConfig config.AgentConfig
	channels    map[string]chan []byte
}

func NewGenericDispatcher(consumer transport.Consumer, workers *workers.WorkerPool, config config.AgentConfig) *genericDispatcher {
	return &genericDispatcher{
		log:         ctrl.Log.WithName("spec-bundle-dispatcher"),
		consumer:    consumer,
		workerPool:  workers,
		agentConfig: config,
		channels:    make(map[string]chan []byte),
	}
}

func (d *genericDispatcher) RegisterChannel(messageID string, channel chan []byte) {
	d.channels[messageID] = channel
	d.log.Info("dispatch channel is registered", "messageID", messageID)
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
		default:
			message := d.consumer.AcquireMessage() // blocking if no message has received
			if message.Destination != d.agentConfig.LeafHubName {
				continue
			}
			channel, found := d.channels[message.ID]
			if !found {
				d.log.Info("dispatching to the default generic channel", "messageID", message.ID)
				channel = d.channels[syncers.GenericMessageKey]
			}
			channel <- message.Payload
		}
	}
}
