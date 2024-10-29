package workers

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-global-hub/agent/pkg/configs"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

// worker within the WorkerPool. runs as a goroutine and invokes Jobs.
type Worker struct {
	log      *zap.SugaredLogger
	id       int
	client   client.Client
	jobQueue chan *Job
}

func newWorker(id int, kubeConfig *rest.Config, jobsQueue chan *Job) (*Worker, error) {
	client, err := client.New(kubeConfig, client.Options{Scheme: configs.GetRuntimeScheme()})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize worker - %w", err)
	}
	return newWorkerWithClient(id, "", client, jobsQueue), nil
}

func newWorkerWithClient(id int, client_id string, k8sClient client.Client, jobsQueue chan *Job) *Worker {
	log := logger.ZapLogger(fmt.Sprintf("spec worker %d", id))
	if client_id != "" {
		log = logger.ZapLogger(fmt.Sprintf("spec worker %s", client_id))
	}
	return &Worker{
		log:      log,
		id:       id,
		client:   k8sClient,
		jobQueue: jobsQueue,
	}
}

func (worker *Worker) start(ctx context.Context) {
	go func() {
		worker.log.Infow("start running worker", "Id: ", worker.id)
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
