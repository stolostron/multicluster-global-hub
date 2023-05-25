package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	localComplianceJobName string = "local-compliance-job"
	startTime              time.Time
	simulationCounter      int64 = 1
)

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, enableSimulation bool, job gocron.Job) {
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
	totalCount, insertedCount, err := syncToLocalComplianceHistory(ctx, pool, 1000, enableSimulation)
	if err != nil {
		log.Error(err, "sync to local_status.compliance_history failed")
	}

	log.Info("finish running", "totalCount", totalCount, "insertedCount", insertedCount)
}

func syncToLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool, batchSize int64, enableSimulation bool,
) (totalCount int64, insertedCount int64, err error) {
	// NOTE: since we get data from event.local_policies instead of local_status.compliance, we don't need to the view
	// create materialized view
	// viewName := fmt.Sprintf("local_status.compliance_view_%s",
	// 	time.Now().AddDate(0, 0, -1).Format("2006_01_02"))
	// createViewSQL := `
	// 	CREATE MATERIALIZED VIEW IF NOT EXISTS %s AS
	// 	SELECT id,cluster_id,compliance
	// 	FROM local_status.compliance;
	// 	CREATE INDEX IF NOT EXISTS idx_local_compliance_view ON %s (id, cluster_id);`
	// _, err = pool.Exec(ctx, fmt.Sprintf(createViewSQL, viewName, viewName))
	// if err != nil {
	// 	return totalCount, insertedCount, err
	// }
	// refresh the materialized view
	// _, err = conn.Exec(ctx, "REFRESH MATERIALIZED VIEW local_compliance_mv")
	// if err != nil {
	// 	return totalCount, syncedCount, err
	// }
	totalCountSQLTemplate := `
		SELECT COUNT(*) FROM (
			SELECT DISTINCT policy_id, cluster_id FROM event.local_policies
			WHERE created_at BETWEEN (CURRENT_DATE - INTERVAL '%d day') AND CURRENT_DATE
		) AS subquery
	`
	beforeToday := int64(1)
	if enableSimulation {
		beforeToday = simulationCounter
	}
	totalCountStatement := fmt.Sprintf(totalCountSQLTemplate, beforeToday)

	if err := pool.QueryRow(ctx, totalCountStatement).Scan(&totalCount); err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistory(ctx, pool, totalCount, batchSize, offset, enableSimulation)
		if err != nil {
			return totalCount, insertedCount, err
		}
		insertedCount += count
	}

	// success, drop the materialized view if exists
	// _, err = pool.Exec(ctx, fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName))
	// if err != nil {
	// 	return totalCount, insertedCount, err
	// }

	if enableSimulation {
		simulationCounter = simulationCounter + 1
	}

	return totalCount, insertedCount, nil
}

func insertToLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool,
	totalCount, batchSize, offset int64, enableSimulation bool,
) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		// insert data with transaction
		conn, err := pool.Acquire(ctx)
		if err != nil {
			fmt.Printf("acquire failed: %v, retrying\n", err)
			return false, nil
		}
		defer conn.Release()

		tx, err := conn.Begin(ctx) // the pool.Begin(ctx) will automatically release the connection after tx.Commit()
		if err != nil {
			fmt.Printf("begin failed: %v, retrying\n", err)
			return false, nil
		}

		// Defer rollback in case of error
		var insertError error
		defer func() {
			if insertError != nil {
				if e := tx.Rollback(ctx); e != nil {
					fmt.Printf("rollback failed: %v\n", e)
				}
			}
			if e := traceComplianceHistory(ctx, pool, localComplianceJobName, totalCount, offset, insertCount,
				startTime, insertError); e != nil {
				fmt.Printf("trace compliance job failed: %v\n", e)
			}
		}()
		selectInsertSQLTemplate := `
			INSERT INTO local_status.compliance_history (id, cluster_id, compliance_date, compliance, 
					compliance_changed_frequency)
			WITH compliance_aggregate AS (
					SELECT cluster_id, policy_id,
							CASE
									WHEN bool_and(compliance = 'compliant') THEN 'compliant'
									ELSE 'non_compliant'
							END::local_status.compliance_type AS aggregated_compliance
					FROM event.local_policies
					WHERE created_at BETWEEN (CURRENT_DATE - INTERVAL '%d day') AND CURRENT_DATE
					GROUP BY cluster_id, policy_id
			)
			SELECT policy_id, cluster_id, (CURRENT_DATE - INTERVAL '%d day'), aggregated_compliance,
					(SELECT COUNT(*) FROM (
							SELECT created_at, compliance, 
									LAG(compliance) OVER (PARTITION BY cluster_id, policy_id ORDER BY created_at ASC)
									AS prev_compliance
							FROM event.local_policies lp
							WHERE (lp.created_at BETWEEN (CURRENT_DATE - INTERVAL '%d day') AND CURRENT_DATE) 
									AND lp.cluster_id = ca.cluster_id AND lp.policy_id = ca.policy_id
							ORDER BY created_at ASC
					) AS subquery WHERE compliance <> prev_compliance) AS compliance_changed_frequency
			FROM compliance_aggregate ca
			ORDER BY cluster_id, policy_id
			LIMIT $1 OFFSET $2`
		beforeTody := int64(1)
		if enableSimulation {
			beforeTody = simulationCounter
		}
		selectInsertStatement := fmt.Sprintf(selectInsertSQLTemplate, beforeTody, beforeTody, beforeTody)
		result, insertError := tx.Exec(ctx, selectInsertStatement, batchSize, offset)
		if insertError != nil {
			fmt.Printf("exec failed: %v, retrying\n", insertError)
			return false, nil
		}
		if insertError = tx.Commit(ctx); insertError != nil {
			fmt.Printf("commit failed: %v, retrying\n", insertError)
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
