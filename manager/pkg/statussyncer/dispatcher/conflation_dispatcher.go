package dispatcher

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/conflator/workerpool"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
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

func AddConflationDispatcher(mgr ctrl.Manager, conflationManager *conflator.ConflationManager,
	managerConfig *config.ManagerConfig, stats *statistics.Statistics,
) error {
	// add work pool: database layer initialization - worker pool + connection pool
	dbWorkerPool, err := workerpool.NewDBWorkerPool(stats)
	if err != nil {
		return fmt.Errorf("failed to initialize DBWorkerPool: %w", err)
	}
	if err := mgr.Add(dbWorkerPool); err != nil {
		return fmt.Errorf("failed to add DB worker pool: %w", err)
	}

	// conflation dispatcher -> work pool
	conflationDispatcher := &ConflationDispatcher{
		log:                  ctrl.Log.WithName("conflation-dispatcher"),
		conflationReadyQueue: conflationManager.GetReadyQueue(),
		dbWorkerPool:         dbWorkerPool,
	}
	if err := mgr.Add(conflationDispatcher); err != nil {
		return fmt.Errorf("failed to add conflation dispatcher: %w", err)
	}
	return nil
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
	var conflationUnit *conflator.ConflationUnit
	for {
		select {
		case <-ctx.Done(): // if dispatcher was stopped do not process more bundles
			return

		default: // as long as context wasn't cancelled, continue and try to read bundles to process
			if conflationUnit == nil {
				conflationUnit = dispatcher.conflationReadyQueue.BlockingDequeue() // blocking if no CU has ready bundle
			}
			dbWorker, err := dispatcher.dbWorkerPool.Acquire()
			if err != nil {
				dispatcher.log.Error(err, "failed to get worker")
				continue
			}

			event, eventMetadata, handleFunc, err := conflationUnit.GetNext()
			if err != nil {
				dispatcher.log.Info(err.Error()) // don't need to throw the error when bundle is not ready
				conflationUnit = nil
				continue
			}

			dbWorker.RunAsync(workerpool.NewDBJob(event, eventMetadata, handleFunc, conflationUnit))
			conflationUnit = nil
		}
	}
}
