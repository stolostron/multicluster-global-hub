package workers

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/stolostron/hub-of-hubs/agent/pkg/spec/controller/rbac"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

// Workpool pool that creates all k8s workers and the assigns k8s jobs to available workers.
type WorkerPool struct {
	ctx                        context.Context
	log                        logr.Logger
	kubeConfig                 *rest.Config
	jobsQueue                  chan *Job
	poolSize                   int
	initializationWaitingGroup sync.WaitGroup
	impersonationManager       *rbac.ImpersonationManager
	impersonationWorkersQueues map[string]chan *Job
	impersonationWorkersLock   sync.Mutex
}

// AddK8sWorkerPool adds k8s workers pool to the manager and returns it.
func AddWorkerPool(log logr.Logger, workpoolSize int, manager ctrl.Manager) (*WorkerPool, error) {
	config := manager.GetConfig()

	// for impersonation workers we have additional workers, one per impersonated user.
	workerPool := &WorkerPool{
		log:                        log,
		kubeConfig:                 config,
		jobsQueue:                  make(chan *Job, workpoolSize), // each worker can handle at most one job at a time
		poolSize:                   workpoolSize,
		initializationWaitingGroup: sync.WaitGroup{},
		impersonationManager:       rbac.NewImpersonationManager(config),
		impersonationWorkersQueues: make(map[string]chan *Job),
		impersonationWorkersLock:   sync.Mutex{},
	}

	workerPool.initializationWaitingGroup.Add(1)

	if err := manager.Add(workerPool); err != nil {
		return nil, fmt.Errorf("failed to initialize k8s workers pool - %w", err)
	}

	return workerPool, nil
}

// Start function starts the k8s workers pool.
func (pool *WorkerPool) Start(ctx context.Context) error {
	pool.ctx = ctx
	pool.initializationWaitingGroup.Done() // once context is saved, it's safe to let RunAsync work with no concerns.

	for i := 1; i <= pool.poolSize; i++ {
		worker, err := newWorker(pool.log, i, pool.kubeConfig, pool.jobsQueue)
		if err != nil {
			return fmt.Errorf("failed to start k8s workers pool - %w", err)
		}

		worker.start(ctx)
	}

	<-ctx.Done() // blocking wait for stop event

	// context was cancelled, do cleanup
	for _, workerQueue := range pool.impersonationWorkersQueues {
		close(workerQueue)
	}

	close(pool.jobsQueue)

	return nil
}

func (pool *WorkerPool) Submit(job *Job) {
	pool.initializationWaitingGroup.Wait() // start running jobs only after some initialization steps have finished.

	userIdentity, err := pool.impersonationManager.GetUserIdentity(job.obj)
	if err != nil {
		pool.log.Error(err, "failed to get user identity from obj")
		return
	}
	// if it doesn't contain impersonation info, let the controller worker pool handle it.
	if userIdentity == rbac.NoIdentity {
		pool.jobsQueue <- job
		return
	}
	// otherwise, need to impersonate and use the specific worker to enforce permissions.
	base64UserGroups, userGroups, err := pool.impersonationManager.GetUserGroups(job.obj)
	if err != nil {
		pool.log.Error(err, "failed to get user groups from obj")
		return
	}

	pool.impersonationWorkersLock.Lock()

	workerIdentifier := fmt.Sprintf("%s.%s", userIdentity, base64UserGroups)

	if _, found := pool.impersonationWorkersQueues[workerIdentifier]; !found {
		if err := pool.createUserWorker(userIdentity, userGroups, workerIdentifier); err != nil {
			pool.log.Error(err, "failed to create user worker", "user", userIdentity)
			return
		}
	}
	// push the job to the queue of the specific worker that uses the user identity
	workerQueue := pool.impersonationWorkersQueues[workerIdentifier]

	pool.impersonationWorkersLock.Unlock()
	workerQueue <- job // since this call might get blocking, first Unlock, then try to insert job into queue
}

func (pool *WorkerPool) createUserWorker(userIdentity string, userGroups []string, workerIdentifier string) error {
	k8sClient, err := pool.impersonationManager.Impersonate(userIdentity, userGroups)
	if err != nil {
		return fmt.Errorf("failed to impersonate - %w", err)
	}

	workerQueue := make(chan *Job, pool.poolSize)
	worker := newWorkerWithClient(pool.log.WithName(fmt.Sprintf("impersonation-%s", userIdentity)), 1, k8sClient, workerQueue)
	worker.start(pool.ctx)
	pool.impersonationWorkersQueues[workerIdentifier] = workerQueue

	return nil
}
