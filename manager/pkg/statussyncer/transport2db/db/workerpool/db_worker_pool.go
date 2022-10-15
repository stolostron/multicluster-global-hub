package workerpool

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db"
	"github.com/stolostron/multicluster-global-hub/manager/pkg/statussyncer/transport2db/db/postgresql"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// NewDBWorkerPool returns a new db workers pool dispatcher.
func NewDBWorkerPool(log logr.Logger, databaseURL string, statistics *statistics.Statistics) (*DBWorkerPool, error) {
	ctx, cancelContext := context.WithCancel(context.Background())

	dbConnPool, err := postgresql.NewPostgreSQL(ctx, databaseURL)
	if err != nil {
		cancelContext()
		return nil, fmt.Errorf("failed to initialize db worker pool - %w", err)
	}

	return &DBWorkerPool{
		ctx:           ctx,
		log:           log,
		cancelContext: cancelContext,
		dbConnPool:    dbConnPool,
		dbWorkers:     make(chan *DBWorker, dbConnPool.GetPoolSize()),
		statistics:    statistics,
	}, nil
}

// DBWorkerPool pool that registers all db workers and the assigns db jobs to available workers.
type DBWorkerPool struct {
	ctx           context.Context
	log           logr.Logger
	cancelContext context.CancelFunc
	dbConnPool    db.StatusTransportBridgeDB
	dbWorkers     chan *DBWorker // A pool of workers that are registered within the workers pool
	statistics    *statistics.Statistics
}

// Start function starts the db workers pool.
func (pool *DBWorkerPool) Start() error {
	var i int32
	// start workers and register them within the workers pool
	for i = 1; i <= pool.dbConnPool.GetPoolSize(); i++ {
		worker := NewDBWorker(pool.log, i, pool.dbWorkers, pool.dbConnPool, pool.statistics)
		worker.start(pool.ctx) // each worker adds itself to the pool inside start function
	}

	return nil
}

// Stop function stops the DBWorker queue.
func (pool *DBWorkerPool) Stop() {
	pool.cancelContext() // worker pool is responsible for stopping it's workers, it's done using context
	pool.dbConnPool.Stop()

	close(pool.dbWorkers)
}

// Acquire tries to acquire an available worker. if no worker is available, blocking until a worker becomes available.
func (pool *DBWorkerPool) Acquire() *DBWorker {
	pool.statistics.SetNumberOfAvailableDBWorkers(len(pool.dbWorkers))
	return <-pool.dbWorkers
}
