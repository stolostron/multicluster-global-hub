package workerpool

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/config"
	"github.com/stolostron/multicluster-global-hub/pkg/conflator/db/postgres"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/statistics"
)

// DBWorkerPool pool that registers all db workers and the assigns db jobs to available workers.
type DBWorkerPool struct {
	log        logr.Logger
	statistics *statistics.Statistics
	dataConfig *config.DatabaseConfig
	connPool   postgres.StatusTransportBridgeDB
	workers    chan *Worker // A pool of workers that are registered within the workers pool
	ticker     *time.Ticker
}

// NewDBWorkerPool returns a new db workers pool dispatcher.
func NewDBWorkerPool(dataConfig *config.DatabaseConfig, statistics *statistics.Statistics) (*DBWorkerPool, error) {
	return &DBWorkerPool{
		log:        ctrl.Log.WithName("worker-pool"),
		dataConfig: dataConfig,
		statistics: statistics,
		ticker:     time.NewTicker(10 * time.Second),
	}, nil
}

// Start function starts the db workers pool.
func (pool *DBWorkerPool) Start(ctx context.Context) error {
	// initialize db connection pool, will be deprecated if the gorm library is used
	dbConnPool, err := postgres.NewStatusPostgreSQL(ctx, &database.DatabaseConfig{
		URL:        pool.dataConfig.ProcessDatabaseURL,
		CaCertPath: pool.dataConfig.CACertPath,
		PoolSize:   pool.dataConfig.MaxOpenConns,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize db worker pool - %w", err)
	}
	pool.connPool = dbConnPool

	// initialize workers pool
	pool.workers = make(chan *Worker, dbConnPool.GetPoolSize())

	// start workers and register them within the workers pool
	var i int32
	for i = 1; i <= pool.connPool.GetPoolSize(); i++ {
		worker := NewWorker(pool.log, i, pool.workers, pool.connPool, pool.statistics)
		go worker.start(ctx) // each worker adds itself to the pool inside start function
	}

	<-ctx.Done() // blocking wait until getting context cancel event

	pool.ticker.Stop()
	pool.connPool.Stop()
	close(pool.workers)

	return nil
}

// Acquire tries to acquire an available worker. if no worker is available, blocking until a worker becomes available.
func (pool *DBWorkerPool) Acquire() (*Worker, error) {
	pool.statistics.SetNumberOfAvailableDBWorkers(len(pool.workers))
	// blocking wait until a worker becomes available or timeout (60 seconds = 6 * 10 seconds)
	for i := 0; i < 6; i += 1 {
		select {
		case res := <-pool.workers:
			return res, nil
		case <-pool.ticker.C:
			pool.log.Info("the db workers are not available, retrying", "seconds", i*10+10)
			continue
		}
	}
	return nil, fmt.Errorf("timeout to get the DBWorker")
}
