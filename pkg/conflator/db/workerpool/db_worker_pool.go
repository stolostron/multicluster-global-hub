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

// NewDBWorkerPool returns a new db workers pool dispatcher.
func NewDBWorkerPool(dataConfig *config.DatabaseConfig, statistics *statistics.Statistics) (*DBWorkerPool, error) {
	return &DBWorkerPool{
		log:        ctrl.Log.WithName("db-worker-pool"),
		dataConfig: dataConfig,
		statistics: statistics,
	}, nil
}

// DBWorkerPool pool that registers all db workers and the assigns db jobs to available workers.
type DBWorkerPool struct {
	log        logr.Logger
	dataConfig *config.DatabaseConfig
	dbConnPool postgres.StatusTransportBridgeDB
	dbWorkers  chan *DBWorker // A pool of workers that are registered within the workers pool
	statistics *statistics.Statistics
}

// Start function starts the db workers pool.
func (pool *DBWorkerPool) Start(ctx context.Context) error {
	dbConnPool, err := postgres.NewStatusPostgreSQL(ctx, &database.DatabaseConfig{
		URL:        pool.dataConfig.ProcessDatabaseURL,
		CaCertPath: pool.dataConfig.CACertPath,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize db worker pool - %w", err)
	}
	pool.dbConnPool = dbConnPool
	pool.dbWorkers = make(chan *DBWorker, dbConnPool.GetPoolSize())

	var i int32
	// start workers and register them within the workers pool
	for i = 1; i <= pool.dbConnPool.GetPoolSize(); i++ {
		worker := NewDBWorker(pool.log, i, pool.dbWorkers, pool.dbConnPool, pool.statistics)
		go worker.start(ctx) // each worker adds itself to the pool inside start function
	}

	<-ctx.Done() // blocking wait until getting context cancel event

	pool.dbConnPool.Stop()
	close(pool.dbWorkers)

	return nil
}

// Acquire tries to acquire an available worker. if no worker is available, blocking until a worker becomes available.
func (pool *DBWorkerPool) Acquire() (*DBWorker, error) {
	pool.statistics.SetNumberOfAvailableDBWorkers(len(pool.dbWorkers))
	select {
	case res := <-pool.dbWorkers:
		return res, nil
	case <-time.After(10 * time.Second):
		return nil, fmt.Errorf("timeout to get the DBWorker")
	}
}
