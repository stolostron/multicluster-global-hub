package dispatcher

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db/workerpool"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator"
)

// NewConflationDispatcher creates a new instance of Dispatcher.
func NewConflationDispatcher(log logr.Logger, conflationReadyQueue *conflator.ConflationReadyQueue,
	dbWorkerPool *workerpool.DBWorkerPool,
) *ConflationDispatcher {
	return &ConflationDispatcher{
		log:                  log,
		conflationReadyQueue: conflationReadyQueue,
		dbWorkerPool:         dbWorkerPool,
	}
}

// ConflationDispatcher abstracts the dispatching of db jobs to db workers. this is done by reading ready CU
// and getting from them a ready to process bundles.
type ConflationDispatcher struct {
	log                  logr.Logger
	conflationReadyQueue *conflator.ConflationReadyQueue
	dbWorkerPool         *workerpool.DBWorkerPool
}

// Start starts the dispatcher.
func (dispatcher *ConflationDispatcher) Start(ctx context.Context) error {
	dispatcher.log.Info("starting dispatcher")

	go dispatcher.dispatch(ctx)

	<-ctx.Done() // blocking wait until getting context cancel event
	dispatcher.log.Info("stopped dispatcher")

	return nil
}

func (dispatcher *ConflationDispatcher) dispatch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done(): // if dispatcher was stopped do not process more bundles
			return

		default: // as long as context wasn't cancelled, continue and try to read bundles to process
			conflationUnit := dispatcher.conflationReadyQueue.BlockingDequeue() // blocking if no CU has ready bundle
			dbWorker, err := dispatcher.dbWorkerPool.Acquire()
			if err != nil {
				dispatcher.log.Error(err, "failed to get dbWorker")
				continue
			}

			bundle, bundleMetadata, handlerFunction, err := conflationUnit.GetNext()
			if err != nil {
				dispatcher.log.Error(err, "failed to get next bundle")
				continue
			}

			dbWorker.RunAsync(workerpool.NewDBJob(bundle, bundleMetadata, handlerFunction, conflationUnit))
		}
	}
}
