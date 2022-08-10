package dbsyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/stolostron/hub-of-hubs/manager/pkg/specsyncer/db2transport/intervalpolicy"
)

type genericDBToTransportSyncer struct {
	log            logr.Logger
	intervalPolicy intervalpolicy.IntervalPolicy
	syncBundleFunc func(ctx context.Context) (bool, error)
}

func (syncer *genericDBToTransportSyncer) Start(ctx context.Context) error {
	syncer.log.Info("initialized syncer")

	if _, err := syncer.syncBundleFunc(ctx); err != nil {
		syncer.log.Error(err, "failed to sync bundle")
	}

	go syncer.periodicSync(ctx)

	<-ctx.Done() // blocking wait for cancel context event
	syncer.log.Info("stopped syncer")

	return nil
}

func (syncer *genericDBToTransportSyncer) periodicSync(ctx context.Context) {
	ticker := time.NewTicker(syncer.intervalPolicy.GetInterval())

	for {
		select {
		case <-ctx.Done(): // we have received a signal to stop
			ticker.Stop()
			return

		case <-ticker.C:
			// define timeout of max sync interval on the sync function
			ctxWithTimeout, cancelFunc := context.WithTimeout(ctx, syncer.intervalPolicy.GetMaxInterval())

			synced, err := syncer.syncBundleFunc(ctxWithTimeout)
			if err != nil {
				syncer.log.Error(err, "failed to sync bundle")
			}

			cancelFunc() // cancel child ctx and is used to cleanup resources once context expires or sync is done.

			// get current sync interval
			currentInterval := syncer.intervalPolicy.GetInterval()

			// notify policy whether sync was actually performed or skipped
			if synced {
				syncer.intervalPolicy.Evaluate()
			} else {
				syncer.intervalPolicy.Reset()
			}

			// get reevaluated sync interval
			reevaluatedInterval := syncer.intervalPolicy.GetInterval()

			// reset ticker if needed
			if currentInterval != reevaluatedInterval {
				ticker.Reset(reevaluatedInterval)
				syncer.log.Info(fmt.Sprintf("sync interval has been reset to %s", reevaluatedInterval.String()))
			}
		}
	}
}
