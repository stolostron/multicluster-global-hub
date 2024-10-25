package dispatcher

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator/workerpool"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewConflationDispatcher creates a new instance of Dispatcher.
func NewConflationDispatcher(conflationReadyQueue *conflator.ConflationReadyQueue,
	dbWorkerPool *workerpool.DBWorkerPool,
) *ConflationDispatcher {
	return &ConflationDispatcher{
		log:                  logger.DefaultZapLogger(),
		conflationReadyQueue: conflationReadyQueue,
		dbWorkerPool:         dbWorkerPool,
	}
}

func AddConflationDispatcher(mgr ctrl.Manager, conflationManager *conflator.ConflationManager,
	managerConfig *configs.ManagerConfig, stats *statistics.Statistics,
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
		log:                  logger.DefaultZapLogger(),
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
	log                  *zap.SugaredLogger
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

		case deltaEventJob := <-dispatcher.conflationReadyQueue.DeltaEventJobChan:
			worker := dispatcher.getBlockingWorker(ctx)
			worker.RunAsync(deltaEventJob)
		case conflationUnit := <-dispatcher.conflationReadyQueue.ConflationUnitChan:
			eventJob, err := conflationUnit.GetNext()
			if err != nil {
				dispatcher.log.Info(err.Error()) // don't need to throw the error when bundle is not ready
				continue
			}
			worker := dispatcher.getBlockingWorker(ctx)
			worker.RunAsync(eventJob)
		}
	}
}

func (dispatcher *ConflationDispatcher) getBlockingWorker(ctx context.Context) (worker *workerpool.Worker) {
	_ = wait.PollUntilContextCancel(ctx, 10*time.Second, true, func(ctx context.Context) (done bool, err error) {
		worker, err = dispatcher.dbWorkerPool.Acquire()
		if err != nil {
			dispatcher.log.Info("retrying to acquire db worker", "error", err)
			return false, nil
		}
		return true, nil
	})
	return worker
}
