package workerpool

import (
	"context"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/status/conflator"
	"github.com/stolostron/multicluster-global-hub/pkg/bundle/version"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/enum"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewWorker creates a new instance of DBWorker.
// jobsQueue is initialized with capacity of 1. this is done in order to make sure dispatcher isn't blocked when calling
// to RunAsync, otherwise it will yield cpu to other go routines.
func NewWorker(workerID int32, dbWorkersPool chan *Worker,
	statistics *statistics.Statistics,
) *Worker {
	return &Worker{
		workerID:   workerID,
		workers:    dbWorkersPool,
		jobsQueue:  make(chan *conflator.ConflationJob, 1),
		statistics: statistics,
	}
}

// Worker worker within the DB Worker pool. runs as a goroutine and invokes DBJobs.
type Worker struct {
	workerID   int32
	workers    chan *Worker
	jobsQueue  chan *conflator.ConflationJob
	statistics *statistics.Statistics
}

// RunAsync runs DBJob and reports status to the given CU. once the job processing is finished worker returns to the
// worker pool in order to run more jobs.
func (worker *Worker) RunAsync(job *conflator.ConflationJob) {
	worker.jobsQueue <- job
}

func (worker *Worker) start(ctx context.Context) {
	log.Infow("started worker", "WorkerID", worker.workerID)
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

func (worker *Worker) handleJob(ctx context.Context, job *conflator.ConflationJob) {
	conn := database.GetConn()

	err := database.Lock(conn)

	defer database.Unlock(conn)

	if err != nil {
		log.Error(err)
		return
	}
	// deprecated: previous full bundle handling
	if job.ElementState == nil {
		worker.fullBundleHandle(ctx, job)
		return
	}

	// the list/watch bundle: only process the current event if previous is finished
	job.ElementState.HandlerLock.Lock()
	defer job.ElementState.HandlerLock.Unlock()

	// based on the handle result, update the element state
	startTime := time.Now()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 1*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			err := job.Handle(ctx, job.Event)
			if err != nil {
				// TODO: This is to handle the expired array bundles from 1.5 to 1.6 upgrade.
				// It will be removed after the upgrade.
				if !strings.Contains(err.Error(), "cannot unmarshal array into Go value of") {
					log.Warnf("received the expired event array bundle %, skipping the event", job.Event.Type())
					return true, nil
				}
				log.Errorf("retrying to handle failed event (%s): %v", job.Event.Type(), err)
				return false, nil
			}

			// mark the offset in kafka position
			job.ElementState.LastProcessedMetadata = job.Metadata

			// update the element state
			job.ElementState.LastProcessedVersion = job.Metadata.Version()

			return true, nil
		},
	)

	worker.statistics.AddDatabaseMetrics(job.Event, time.Since(startTime), err)

	if err != nil {
		log.Errorw("fails to process the DB job", "LF", job.Event.Source(), "WorkerID", worker.workerID,
			"event", job.Event, "error", err)
	} else {
		log.Debugw("handle the DB job successfully", "LF", job.Event.Source(),
			"WorkerID", worker.workerID,
			"version", job.Event.Extensions()[version.ExtVersion])
	}
}

func (worker *Worker) fullBundleHandle(ctx context.Context, job *conflator.ConflationJob) {
	startTime := time.Now()
	// handle the event until it's metadata is marked as processed
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 5*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			err := job.Handle(ctx, job.Event) // db connection released to pool when done
			if err != nil {
				job.Metadata.MarkAsUnprocessed()
				log.Warnf("failed to handle event (%s): %v", job.Event.Type(), err)
			} else {
				job.Metadata.MarkAsProcessed()
			}
			// retrying
			if !job.Metadata.Processed() || err != nil {
				log.Infof("retrying to handle the job event: %v", job.Metadata)
				return false, nil
			}
			// success or up to retry threshold
			return job.Metadata.Processed(), nil
		})

	worker.statistics.AddDatabaseMetrics(job.Event, time.Since(startTime), err)

	job.Reporter.ReportResult(job.Metadata, err)

	if err != nil {
		log.Error(err, "fails to process the DB job", "LF", job.Event.Source(),
			"WorkerID", worker.workerID,
			"type", enum.ShortenEventType(job.Event.Type()),
			"version", job.Metadata.Version())
	} else {
		log.Debugw("handle the DB job successfully", "LF", job.Event.Source(),
			"WorkerID", worker.workerID,
			"type", enum.ShortenEventType(job.Event.Type()),
			"version", job.Metadata.Version())
	}
}
