package task

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v4/pgxpool"
)

var batchSize = 1000

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, job gocron.Job) {
	log := ctrl.Log.WithName("local-compliance-job")
	log.Info("start running", "LastRun", job.LastRun().Format("2006-01-02 15:04:05"),
		"NextRun", job.NextRun().Format("2006-01-02 15:04:05"))

	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Error(err, "acquire connection failed")
	}
	defer conn.Release()

	// create view
	_, err = conn.Exec(ctx, `
	CREATE OR REPLACE VIEW local_compliance_view AS 
		SELECT id,cluster_id,compliance 
		FROM local_status.compliance`)
	if err != nil {
		log.Error(err, "create local_compliance_view failed")
	}
	defer func(ctx context.Context) {
		_, err = conn.Exec(ctx, `DROP VIEW IF EXISTS local_compliance_view`)
		if err != nil {
			log.Error(err, "drop local_compliance_view failed")
		}
	}(ctx)

	// batch insert
	var count int64
	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", "local_compliance_view")).Scan(&count)
	if err != nil {
		log.Error(err, "query count failed")
	}
	log.Info("total rows", "count", count)

	for offset := 0; offset < int(count); offset += batchSize {
		// retry until success, use timeout context to avoid long running
		timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
			if err := syncBatchData(ctx, conn, batchSize, offset, log); err != nil {
				log.Error(err, "retry to sync data", "batchSize", batchSize, "offset", offset)
				return false, nil
			}
			return true, nil
		}); err != nil {
			log.Error(err, "sync data failed", "batchSize", batchSize, "offset", offset)
		}
	}
	log.Info("finish running")
}

func syncBatchData(ctx context.Context, conn *pgxpool.Conn, batchSize, offset int, log logr.Logger) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `
			INSERT INTO local_status.compliance_history (id, cluster_id, compliance) 
			SELECT id,cluster_id,compliance 
			FROM local_compliance_view LIMIT $1 OFFSET $2`, batchSize, offset)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	} else {
		log.Info("insert rows", "batchSize", batchSize, "offset", offset, "insertCount", result.RowsAffected())
		return nil
	}
}
