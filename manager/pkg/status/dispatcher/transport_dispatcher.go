package dispatcher

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
)

// Get message from transport, convert it to bundle and forward it to conflation manager.
type TransportDispatcher struct {
	log               *zap.SugaredLogger
	consumer          transport.Consumer
	conflationManager *conflator.ConflationManager
	statistic         *statistics.Statistics
}

func AddTransportDispatcher(mgr ctrl.Manager, consumer transport.Consumer, managerConfig *configs.ManagerConfig,
	conflationManager *conflator.ConflationManager, stats *statistics.Statistics,
) error {
	transportDispatcher := &TransportDispatcher{
		log:               logger.DefaultZapLogger(),
		consumer:          consumer,
		conflationManager: conflationManager,
		statistic:         stats,
	}
	if err := mgr.Add(transportDispatcher); err != nil {
		return fmt.Errorf("failed to add transport dispatcher to runtime manager: %w", err)
	}
	return nil
}

// Start function starts bundles status syncer.
func (d *TransportDispatcher) Start(ctx context.Context) error {
	d.log.Info("transport dispatcher starts dispatching received events...")

	go d.dispatch(ctx)

	<-ctx.Done() // blocking wait for stop event
	d.log.Info("stopped dispatching events")

	return nil
}

func (d *TransportDispatcher) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-d.consumer.EventChan():
			d.statistic.ReceivedEvent(evt)
			d.log.Debugw("forward received event to conflation", "event type", enum.ShortenEventType(evt.Type()))
			d.conflationManager.Insert(evt)
		}
	}
}
