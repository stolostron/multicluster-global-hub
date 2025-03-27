package spec

import (
	"context"

	"go.uber.org/zap"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

var addToMgr = false

type genericDispatcher struct {
	log         *zap.SugaredLogger
	consumer    transport.Consumer
	agentConfig configs.AgentConfig
	syncers     map[string]Syncer
}

func AddGenericDispatcher(mgr ctrl.Manager, consumer transport.Consumer, config configs.AgentConfig,
) (Dispatcher, error) {
	if addToMgr {
		return nil, nil
	}
	dispatcher := &genericDispatcher{
		log:         logger.DefaultZapLogger(),
		consumer:    consumer,
		agentConfig: config,
		syncers:     make(map[string]Syncer),
	}
	if err := mgr.Add(dispatcher); err != nil {
		return nil, err
	}

	addToMgr = true
	return dispatcher, nil
}

func (d *genericDispatcher) RegisterSyncer(messageID string, syncer Syncer) {
	d.syncers[messageID] = syncer
	d.log.Infow("dispatch syncer is registered", "messageID", messageID)
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
			d.log.Debugf("get event: %v", evt.Type())
			// if destination is explicitly specified and does not match, drop bundle
			clusterNameVal, err := evt.Context.GetExtension(constants.CloudEventExtensionKeyClusterName)
			if err != nil {
				d.log.Infow("event dropped due to cluster name retrieval error", "error", err)
				continue
			}
			clusterName, ok := clusterNameVal.(string)
			if !ok {
				d.log.Infow("event dropped due to invalid cluster name", "clusterName", clusterNameVal)
				continue
			}
			if clusterName != transport.Broadcast && clusterName != d.agentConfig.LeafHubName {
				// d.log.Infow("event dropped due to cluster name mismatch", "clusterName", clusterName)
				continue
			}
			syncer, found := d.syncers[evt.Type()]
			d.log.Debug("found: %v", found)
			if !found {
				d.log.Debugw("dispatching to the default generic syncer", "eventType", evt.Type())
				syncer = d.syncers[constants.GenericSpecMsgKey]
			}
			if syncer == nil || evt == nil {
				d.log.Warnw("nil syncer or event: incompatible event will be resolved after upgrade.",
					"syncer", syncer, "event", evt)
				continue
			}
			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				d.log.Debug("sync data: %v", evt.Data())
				if err := syncer.Sync(ctx, evt.Data()); err != nil {
					return err
				}
				return nil
			}); err != nil {
				d.log.Errorw("sync failed", "type", evt.Type(), "error", err)
			}
		}
	}
}
