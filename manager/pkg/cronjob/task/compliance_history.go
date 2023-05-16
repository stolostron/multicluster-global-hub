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

var (
	localComplianceJobName string = "local-compliance-job"
	startTime              time.Time
)

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, job gocron.Job) {
	log := ctrl.Log.WithName(localComplianceJobName)

	log.Info("start running", "LastRun", job.LastRun().Format("2006-01-02 15:04:05"),
		"NextRun", job.NextRun().Format("2006-01-02 15:04:05"))
	startTime = time.Now()

	// batchSize = 1000 for now
	// The suitable batchSize for selecting and inserting a lot of records from a table in PostgreSQL depends on
	// several factors such as the size of the table, available memory, network bandwidth, and hardware specifications.
	// However, as a general rule of thumb, a batch size of around 1000 to 5000 records is a good starting point. This
	// size provides a balance between minimizing the number of queries sent to the server while still being efficient
	// and not overloading the system. To determine the optimal batchSize, it may be helpful to test different batch
	// sizes and measure the performance of the queries.
	totalCount, insertedCount, err := syncToLocalComplianceHistory(ctx, pool, 1000)
	if err != nil {
		log.Error(err, "sync to local_status.compliance_history failed")
	}

	// if err := traceComplianceHistory(ctx, conn, jobName, totalCount, syncedCount, start, err); err != nil {
	// 	log.Error(err, "trace job failed")
	// }

	log.Info("finish running", "totalCount", totalCount, "insertedCount", insertedCount)
}

func syncToLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool, batchSize int64,
) (totalCount int64, insertedCount int64, err error) {
	// create materialized view
	viewName := fmt.Sprintf("local_status.compliance_view_%s", time.Now().AddDate(0, 0, -1).Format("2006_01_02"))
	createViewSQL := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS %s AS 
		SELECT id,cluster_id,compliance 
		FROM local_status.compliance;
		CREATE INDEX IF NOT EXISTS idx_local_compliance_view ON %s (id, cluster_id);`
	_, err = pool.Exec(ctx, fmt.Sprintf(createViewSQL, viewName, viewName))
	if err != nil {
		return totalCount, insertedCount, err
	}
	// refresh the materialized view
	// _, err = conn.Exec(ctx, "REFRESH MATERIALIZED VIEW local_compliance_mv")
	// if err != nil {
	// 	return totalCount, syncedCount, err
	// }

	err = pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", viewName)).Scan(&totalCount)
	if err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistory(ctx, pool, viewName, totalCount, batchSize, offset)
		if err != nil {
			return totalCount, insertedCount, err
		}
		insertedCount += count
	}

	// success, drop the materialized view if exists
	_, err = pool.Exec(ctx, fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName))
	if err != nil {
		return totalCount, insertedCount, err
	}

	return totalCount, insertedCount, nil
}

func insertToLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool, viewName string,
	totalCount, batchSize, offset int64,
) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		// insert data with transaction
		tx, err := pool.Begin(ctx)
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
			e := traceComplianceHistory(ctx, pool, localComplianceJobName, totalCount, offset, insertCount, startTime, err)
			if e != nil {
				fmt.Printf("trace compliance job failed: %v\n", e)
			}
		}()
		selectInsertSQL := `
			INSERT INTO local_status.compliance_history (id, cluster_id, compliance) 
			SELECT id,cluster_id,compliance 
			FROM %s 
			ORDER BY id, cluster_id
			LIMIT $1 
			OFFSET $2`
		result, err := tx.Exec(ctx, fmt.Sprintf(selectInsertSQL, viewName), batchSize, offset)
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

func traceComplianceHistory(ctx context.Context, pool *pgxpool.Pool, name string, total, offset, inserted int64,
	start time.Time, err error,
) error {
	end := time.Now()
	errMessage := "none"
	if err != nil {
		errMessage = err.Error()
	}
	_, err = pool.Exec(ctx, `
	INSERT INTO local_status.compliance_history_job_log (name, start_at, end_at, total, offsets, inserted, error) 
	VALUES ($1, $2, $3, $4, $5, $6, $7);`, name, start, end, total, offset, inserted, errMessage)

	return err
}
