package task

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
)

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, job gocron.Job) {
	jobName := "local-compliance-job"
	log := ctrl.Log.WithName(jobName)

	log.Info("start running", "LastRun", job.LastRun().Format("2006-01-02 15:04:05"),
		"NextRun", job.NextRun().Format("2006-01-02 15:04:05"))
	start := time.Now()

	conn, err := pool.Acquire(ctx)
	if err != nil {
		log.Error(err, "acquire connection failed")
	}
	defer conn.Release()

	// batchSize = 1000 for now
	// The suitable batchSize for selecting and inserting a lot of records from a table in PostgreSQL depends on several factors such as the size of the table, available memory, network bandwidth, and hardware specifications. However, as a general rule of thumb, a batch size of around 1000 to 5000 records is a good starting point. This size provides a balance between minimizing the number of queries sent to the server while still being efficient and not overloading the system. To determine the optimal batchSize, it may be helpful to test different batch sizes and measure the performance of the queries.
	totalCount, syncedCount, err := syncToLocalComplianceHistory(ctx, conn, 1000)
	if err != nil {
		log.Error(err, "sync to local_status.compliance_history failed")
	}

	if err := traceJob(ctx, conn, jobName, "local_status.compliance", "local_status.compliance_history",
		totalCount, syncedCount, start, err); err != nil {
		log.Error(err, "trace job failed")
	}

	log.Info("finish running", "totalCount", totalCount, "syncedCount", syncedCount)
}

func syncToLocalComplianceHistory(ctx context.Context, conn *pgxpool.Conn, batchSize int,
) (totalCount int64, syncedCount int64, err error) {
	// create materialized view
	_, err = conn.Exec(ctx, `
		CREATE MATERIALIZED VIEW IF NOT EXISTS local_compliance_mv AS 
			SELECT id,cluster_id,compliance 
			FROM local_status.compliance;
		CREATE INDEX IF NOT EXISTS idx_local_compliance_mv ON local_compliance_mv (id, cluster_id);
		`)
	if err != nil {
		return totalCount, syncedCount, err
	}
	// refresh the materialized view
	_, err = conn.Exec(ctx, "REFRESH MATERIALIZED VIEW local_compliance_mv")
	if err != nil {
		return totalCount, syncedCount, err
	}

	err = conn.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", "local_compliance_mv")).Scan(&totalCount)
	if err != nil {
		return totalCount, syncedCount, err
	}

	for offset := 0; offset < int(totalCount); offset += batchSize {
		insertCount, err := insertToLocalComplianceHistory(ctx, conn, batchSize, offset)
		if err != nil {
			return totalCount, syncedCount, err
		}
		syncedCount += insertCount
	}
	return totalCount, syncedCount, nil
}

func insertToLocalComplianceHistory(ctx context.Context, conn *pgxpool.Conn, batchSize, offset int) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		// insert data with transaction
		tx, err := conn.Begin(ctx)
		if err != nil {
			fmt.Printf("begin failed: %v\n", err)
			return false, nil
		}

		// Defer rollback in case of error
		defer func() {
			if err != nil {
				if e := tx.Rollback(ctx); e != nil {
					fmt.Printf("rollback failed: %v\n", e)
				}
			}
		}()

		result, err := tx.Exec(ctx, `
				INSERT INTO local_status.compliance_history (id, cluster_id, compliance) 
				SELECT id,cluster_id,compliance 
				FROM local_compliance_mv 
				ORDER BY id, cluster_id
				LIMIT $1 
				OFFSET $2`,
			batchSize, offset)
		if err != nil {
			fmt.Printf("exec failed: %v\n", err)
			return false, nil
		}
		if err := tx.Commit(ctx); err != nil {
			fmt.Printf("commit failed: %v\n", err)
			return false, nil
		} else {
			insertCount = result.RowsAffected()
			fmt.Printf("batchSize: %d, insert: %d, offset: %d\n", batchSize, insertCount, offset)
			return true, nil
		}
	})
	return insertCount, err
}

func traceJob(ctx context.Context, conn *pgxpool.Conn, name, source, target string, total, synced int64,
	start time.Time, err error,
) error {
	end := time.Now()
	errMessage := ""
	if err != nil {
		errMessage = err.Error()
	}
	_, err = conn.Exec(ctx, `
	INSERT INTO local_status.job_log (name, start_at, end_at, source, target, synced_count, total_count, error) 
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8);`,
		name, start, end, source, target, synced, total, errMessage)

	return err
}
