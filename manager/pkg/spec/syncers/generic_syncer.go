package syncers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/controllers/bundle"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/specdb"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/spec/syncers/interval"
	"github.com/stolostron/multicluster-global-hub/pkg/constants"
	"github.com/stolostron/multicluster-global-hub/pkg/transport"
	"github.com/stolostron/multicluster-global-hub/pkg/utils"
)

type genericDBToTransportSyncer struct {
	log            *zap.SugaredLogger
	intervalPolicy interval.IntervalPolicy
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

// syncObjectsBundle performs the actual sync logic and returns true if bundle was committed to transport,
// otherwise false.
func syncObjectsBundle(ctx context.Context, producer transport.Producer, eventType string,
	specDB specdb.SpecDB, dbTableName string, createObjFunc bundle.CreateObjectFunction,
	createBundleFunc bundle.CreateBundleFunction, lastSyncTimestampPtr *time.Time,
) (bool, error) {
	lastUpdateTimestamp, err := specDB.GetLastUpdateTimestamp(ctx, dbTableName, true) // filter local resources
	if err != nil {
		return false, fmt.Errorf("unable to sync bundle - %w", err)
	}

	if !lastUpdateTimestamp.After(*lastSyncTimestampPtr) { // sync only if something has changed
		return false, nil
	}

	// if we got here, then the last update timestamp from db is after what we have in memory.
	// this means something has changed in db, syncing all the objects to transport.
	bundleResult := createBundleFunc()
	lastUpdateTimestamp, err = specDB.GetObjectsBundle(ctx, dbTableName, createObjFunc, bundleResult)
	if err != nil {
		return false, fmt.Errorf("unable to sync bundle - %w", err)
	}

	// send message to transport
	payloadBytes, err := json.Marshal(bundleResult)
	if err != nil {
		return false, fmt.Errorf("failed to sync marshal bundle(%s)", eventType)
	}

	evt := utils.ToCloudEvent(eventType, constants.CloudEventGlobalHubClusterName, transport.Broadcast, payloadBytes)
	if err := producer.SendEvent(ctx, evt); err != nil {
		return false, fmt.Errorf("failed to sync message(%s) from table(%s) to destination(%s) - %w",
			eventType, dbTableName, transport.Broadcast, err)
	}

	// updating value to retain same ptr between calls
	*lastSyncTimestampPtr = *lastUpdateTimestamp
	return true, nil
}
