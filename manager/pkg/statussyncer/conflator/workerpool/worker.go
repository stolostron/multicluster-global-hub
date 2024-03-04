package workerpool

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewWorker creates a new instance of DBWorker.
// jobsQueue is initialized with capacity of 1. this is done in order to make sure dispatcher isn't blocked when calling
// to RunAsync, otherwise it will yield cpu to other go routines.
func NewWorker(log logr.Logger, workerID int32, dbWorkersPool chan *Worker,
	statistics *statistics.Statistics,
) *Worker {
	return &Worker{
		log:        log,
		workerID:   workerID,
		workers:    dbWorkersPool,
		jobsQueue:  make(chan *DBJob, 1),
		statistics: statistics,
	}
}

// Worker worker within the DB Worker pool. runs as a goroutine and invokes DBJobs.
type Worker struct {
	log        logr.Logger
	workerID   int32
	workers    chan *Worker
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
			worker.handleJob(ctx, job)
		}
	}
}

func (worker *Worker) handleJob(ctx context.Context, job *DBJob) {
	startTime := time.Now()
	conn := database.GetConn()

	err := database.Lock(conn)

	defer database.Unlock(conn)

	if err != nil {
		worker.log.Error(err, "failed to get db lock")
		return
	}

	// handle the event until it's metadata is marked as processed
	err = wait.PollUntilContextTimeout(ctx, 2*time.Second, 5*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			err = job.handleFunc(ctx, job.event) // db connection released to pool when done
			if err != nil {
				job.eventMetadata.MarkAsUnprocessed()
				worker.log.Error(err, "failed to handle event", "type", job.event.Type())
			} else {
				job.eventMetadata.MarkAsProcessed()
			}
			// retrying
			if !job.eventMetadata.Processed() {
				return false, nil
			}
			// success or up to retry threshold
			return job.eventMetadata.Processed(), err
		})

	worker.statistics.AddDatabaseMetrics(job.event, time.Since(startTime), err)
	job.conflationUnitResultReporter.ReportResult(job.eventMetadata, err)

	if err != nil {
		worker.log.Error(err, "fails to process the DB job", "LF", job.event.Source(),
			"WorkerID", worker.workerID,
			"type", job.event.Type(),
			"version", job.eventMetadata.Version())
	} else {
		worker.log.V(2).Info("handle the DB job successfully", "LF", job.event.Source(),
			"WorkerID", worker.workerID,
			"type", job.event.Type(),
			"version", job.eventMetadata.Version())
	}
}
