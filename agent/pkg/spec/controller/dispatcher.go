package controller

import (
	"context"
	"encoding/json"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/config"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/bundle"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/syncers"
	"github.com/stolostron/multicluster-global-hub/agent/pkg/spec/controller/workers"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/registration"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

type Dispatcher interface {
	Start(ctx context.Context) error
	// RegisterCustomBundle registers a bundle ID with a CustomBundleRegistration.
	// None-registered bundles are assumed to be a GenericBundle.
	RegisterCustomBundle(msgID string, customBundle *registration.CustomBundleRegistration)

	RegisterBundleSyncer(msgID string, syncer syncers.Syncer)
}

type genericDispatcher struct {
	log                       logr.Logger
	consumer                  transport.Consumer
	workerPool                *workers.WorkerPool
	agentConfig               config.AgentConfig
	customBundleRegistrations map[string]*registration.CustomBundleRegistration // register the custom bundle
}

func NewGenericDispatcher(consumer transport.Consumer, workers *workers.WorkerPool, config config.AgentConfig) *genericDispatcher {
	return &genericDispatcher{
		log:                       ctrl.Log.WithName("spec-bundle-dispatcher"),
		consumer:                  consumer,
		workerPool:                workers,
		agentConfig:               config,
		customBundleRegistrations: make(map[string]*registration.CustomBundleRegistration),
	}
}

// Start function starts bundles spec syncer.
func (d *genericDispatcher) Start(ctx context.Context) error {
	d.log.Info("started dispatching received bundles...")

	go d.dispatch(ctx)

	<-ctx.Done() // blocking wait for stop event
	d.log.Info("stopped dispatching bundles")

	return nil
}

// Register a bundle ID with a CustomBundleRegistration. TODO: BundleSyncer()
func (d *genericDispatcher) RegisterCustomBundle(msgID string,
	customBundleRegistration *registration.CustomBundleRegistration,
) {
	d.customBundleRegistrations[msgID] = customBundleRegistration
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
			d.process(message)
		}
	}
}

func (d *genericDispatcher) process(message *transport.Message) {
	// TODO: reigster syncers in the dispatcher
	// dispatcher dispatch the message to specific sycner by message id
	// syncers are managed by the runtime manager

	customBundleRegistration, found := d.customBundleRegistrations[message.ID]
	if found { // received a custom bundle
		receivedBundle := customBundleRegistration.InitBundlesResourceFunc()
		if err := json.Unmarshal(message.Payload, &receivedBundle); err != nil {
			d.log.Error(err, "parse CustomBundle error", "ID", message.ID, "Type", message.MsgType, "Version", message.Version)
		}
		customBundleRegistration.BundleUpdatesChan <- receivedBundle
		return
	}

	// received generic bundle
	receivedBundle := bundle.NewGenericBundle()
	if err := json.Unmarshal(message.Payload, receivedBundle); err != nil {
		d.log.Error(err, "parse GenericBundle error", "ID", message.ID, "Type", message.MsgType, "Version", message.Version)
	}
	// TODO: send message to the genericBundlesChan to process the message
}
