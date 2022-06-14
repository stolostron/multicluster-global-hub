package workers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// worker within the WorkerPool. runs as a goroutine and invokes Jobs.
type Worker struct {
	log      logr.Logger
	id       int
	client   client.Client
	jobQueue chan *Job
}

func newWorker(log logr.Logger, id int, kubeConfig *rest.Config, jobsQueue chan *Job) (*Worker, error) {
	client, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize worker - %w", err)
	}
	return newWorkerWithClient(log, id, client, jobsQueue), nil
}

func newWorkerWithClient(log logr.Logger, id int, k8sClient client.Client, jobsQueue chan *Job) *Worker {
	return &Worker{
		log:      log,
		id:       id,
		client:   k8sClient,
		jobQueue: jobsQueue,
	}
}

func (worker *Worker) start(ctx context.Context) {
	go func() {
		worker.log.Info("start running worker", "Id: ", worker.id)
		for {
			select {
			case <-ctx.Done(): // received a signal to stop
				return
			case job := <-worker.jobQueue: // Worker received a job request.
				job.handlerFunc(ctx, worker.client, job.obj)
			}
		}
	}()
}
