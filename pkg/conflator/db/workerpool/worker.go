package workerpool

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewWorker creates a new instance of DBWorker.
// jobsQueue is initialized with capacity of 1. this is done in order to make sure dispatcher isn't blocked when calling
// to RunAsync, otherwise it will yield cpu to other go routines.
func NewWorker(log logr.Logger, workerID int32, dbWorkersPool chan *Worker,
	dbConnPool postgres.StatusTransportBridgeDB, statistics *statistics.Statistics,
) *Worker {
	return &Worker{
		log:        log,
		workerID:   workerID,
		workers:    dbWorkersPool,
		connPool:   dbConnPool,
		jobsQueue:  make(chan *DBJob, 1),
		statistics: statistics,
	}
}

// Worker worker within the DB Worker pool. runs as a goroutine and invokes DBJobs.
type Worker struct {
	log        logr.Logger
	workerID   int32
	workers    chan *Worker
	connPool   postgres.StatusTransportBridgeDB
	jobsQueue  chan *DBJob
	statistics *statistics.Statistics
}

// RunAsync runs DBJob and reports status to the given CU. once the job processing is finished worker returns to the
// worker pool in order to run more jobs.
func (worker *Worker) RunAsync(job *DBJob) {
	worker.jobsQueue <- job
}

func (worker *Worker) start(ctx context.Context) {
	worker.log.Info("started worker", "WorkerID", worker.workerID)
	for {
		// add worker into the dbWorkerPool to mark this worker as available.
		// this is done in each iteration after the worker finished handling a job (or at startup),
		// for receiving a new job to handle.
		worker.workers <- worker
		worker.statistics.SetNumberOfAvailableDBWorkers(len(worker.workers))

		select {
		case <-ctx.Done(): // we have received a signal to stop
			close(worker.jobsQueue)
			return

		case job := <-worker.jobsQueue: // DBWorker received a job request.
			startTime := time.Now()
			err := job.handlerFunc(ctx, job.bundle, worker.connPool) // db connection released to pool when done
			worker.statistics.AddDatabaseMetrics(job.bundle, time.Since(startTime), err)
			job.conflationUnitResultReporter.ReportResult(job.bundleMetadata, err)

			if err != nil {
				worker.log.Error(err, "worker fails to process the DB job", "LeafHubName", job.bundle.GetLeafHubName(),
					"WorkerID", worker.workerID,
					"BundleType", helpers.GetBundleType(job.bundle),
					"Version", job.bundle.GetVersion().String())
			} else {
				worker.log.Info("worker processes the DB job successfully", "LeafHubName", job.bundle.GetLeafHubName(),
					"WorkerID", worker.workerID,
					"BundleType", helpers.GetBundleType(job.bundle),
					"Version", job.bundle.GetVersion().String())
			}
		}
	}
}
