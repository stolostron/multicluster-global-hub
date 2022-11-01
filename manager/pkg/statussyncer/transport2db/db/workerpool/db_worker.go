package workerpool

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/pkg/bundle/helpers"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewDBWorker creates a new instance of DBWorker.
// jobsQueue is initialized with capacity of 1. this is done in order to make sure dispatcher isn't blocked when calling
// to RunAsync, otherwise it will yield cpu to other go routines.
func NewDBWorker(log logr.Logger, workerID int32, dbWorkersPool chan *DBWorker,
	dbConnPool database.StatusTransportBridgeDB, statistics *statistics.Statistics,
) *DBWorker {
	return &DBWorker{
		log:           log,
		workerID:      workerID,
		dbWorkersPool: dbWorkersPool,
		dbConnPool:    dbConnPool,
		jobsQueue:     make(chan *DBJob, 1),
		statistics:    statistics,
	}
}

// DBWorker worker within the DB Worker pool. runs as a goroutine and invokes DBJobs.
type DBWorker struct {
	log           logr.Logger
	workerID      int32
	dbWorkersPool chan *DBWorker
	dbConnPool    database.StatusTransportBridgeDB
	jobsQueue     chan *DBJob
	statistics    *statistics.Statistics
}

// RunAsync runs DBJob and reports status to the given CU. once the job processing is finished worker returns to the
// worker pool in order to run more jobs.
func (worker *DBWorker) RunAsync(job *DBJob) {
	worker.jobsQueue <- job
}

func (worker *DBWorker) start(ctx context.Context) {
	worker.log.Info("started worker", "WorkerID", worker.workerID)
	for {
		// add worker into the dbWorkerPool to mark this worker as available.
		// this is done in each iteration after the worker finished handling a job (or at startup),
		// for receiving a new job to handle.
		worker.dbWorkersPool <- worker
		worker.statistics.SetNumberOfAvailableDBWorkers(len(worker.dbWorkersPool))

		select {
		case <-ctx.Done(): // we have received a signal to stop
			close(worker.jobsQueue)
			return

		case job := <-worker.jobsQueue: // DBWorker received a job request.
			worker.log.Info("received DB job", "WorkerID", worker.workerID, "BundleType",
				helpers.GetBundleType(job.bundle), "LeafHubName", job.bundle.GetLeafHubName(), "Version",
				job.bundle.GetVersion().String())

			startTime := time.Now()
			err := job.handlerFunc(ctx, job.bundle, worker.dbConnPool) // db connection released to pool when done
			worker.statistics.AddDatabaseMetrics(job.bundle, time.Since(startTime), err)
			job.conflationUnitResultReporter.ReportResult(job.bundleMetadata, err)

			if err != nil {
				worker.log.Error(err, "failed processing DB job", "WorkerID", worker.workerID,
					"BundleType", helpers.GetBundleType(job.bundle),
					"LeafHubName", job.bundle.GetLeafHubName(),
					"Version", job.bundle.GetVersion().String())
			} else {
				worker.log.Info("finished processing DB job", "WorkerID", worker.workerID,
					"BundleType", helpers.GetBundleType(job.bundle),
					"LeafHubName", job.bundle.GetLeafHubName(),
					"Version", job.bundle.GetVersion().String())
			}
		}
	}
}
