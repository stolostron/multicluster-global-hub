package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4/pgxpool"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	localComplianceTaskName = "local-compliance-history"
	startTime               time.Time
	log                     logr.Logger
	timeFormat              = "2006-01-02 15:04:05"
	dateFormat              = "2006-01-02"
	dateInterval            = 1
	simulationCounter       = 1
	batchSize               = int64(1000)
	// batchSize = 1000 for now
	// The suitable batchSize for selecting and inserting a lot of records from a table in PostgreSQL depends on
	// several factors such as the size of the table, available memory, network bandwidth, and hardware specifications.
	// However, as a general rule of thumb, a batch size of around 1000 to 5000 records is a good starting point. This
	// size provides a balance between minimizing the number of queries sent to the server while still being efficient
	// and not overloading the system. To determine the optimal batchSize, it may be helpful to test different batch
	// sizes and measure the performance of the queries.
)

func SyncLocalCompliance(ctx context.Context, pool *pgxpool.Pool, enableSimulation bool, job gocron.Job) {
	startTime = time.Now()

	interval := dateInterval
	if enableSimulation {
		interval = simulationCounter
	}

	historyDate := startTime.AddDate(0, 0, -interval)
	log = ctrl.Log.WithName(localComplianceTaskName).WithValues("history", historyDate.Format(dateFormat))
	log.Info("start running", "currentRun", job.LastRun().Format(timeFormat))

	// insert or update with local_status.compliance
	statusTotal, statusInsert, err := syncToLocalComplianceHistoryByLocalStatus(ctx, pool, batchSize, interval)
	if err != nil {
		log.Error(err, "sync from local_status.compliance to history.local_compliance failed")
		return
	}
	log.Info("with local_status.compliance", "totalCount", statusTotal, "insertedCount", statusInsert)

	if enableSimulation {
		simulationCounter++
		return
	}

	// insert or update with event.local_policies
	eventTotal, eventInsert, err := syncToLocalComplianceHistoryByPolicyEvent(ctx, pool, batchSize)
	if err != nil {
		log.Error(err, "sync from history.local_policies to history.local_compliance failed")
		return
	}
	log.Info("with event.local_policies", "totalCount", eventTotal, "insertedCount", eventInsert)

	log.Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
}

func syncToLocalComplianceHistoryByLocalStatus(ctx context.Context, pool *pgxpool.Pool, batchSize int64, interval int) (
	totalCount int64, insertedCount int64, err error,
) {
	viewSchema := "history"
	viewTable := fmt.Sprintf("local_compliance_view_%s",
		startTime.AddDate(0, 0, -interval).Format("2006_01_02"))
	viewName := fmt.Sprintf("%s.%s", viewSchema, viewTable)
	createViewTemplate := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS %s AS
			SELECT policy_id,cluster_id,leaf_hub_name,compliance 
			FROM local_status.compliance
		WITH DATA;
		CREATE INDEX IF NOT EXISTS idx_local_compliance_view ON %s (policy_id, cluster_id);
	`
	_, err = pool.Exec(ctx, fmt.Sprintf(createViewTemplate, viewName, viewName))
	if err != nil {
		return totalCount, insertedCount, err
	}

	err = pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", viewName)).Scan(&totalCount)
	if err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistoryByLocalStatus(ctx, viewName, interval, pool, totalCount,
			batchSize, offset)
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

func insertToLocalComplianceHistoryByLocalStatus(ctx context.Context, tableName string, interval int,
	pool *pgxpool.Pool, totalCount, batchSize, offset int64,
) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	var err error
	defer func() {
		if e := traceComplianceHistory(ctx, pool,
			fmt.Sprintf("%s/local_status.compliance", localComplianceTaskName),
			totalCount, offset, insertCount, startTime, err); e != nil {
			log.Info("trace compliance job failed, retrying", "error", e)
		}
	}()
	err = wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		selectInsertSQLTemplate := `
			INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance, compliance_date) 
				(
					SELECT policy_id,cluster_id,leaf_hub_name,compliance,(CURRENT_DATE - INTERVAL '%d day') 
					FROM %s 
					ORDER BY policy_id, cluster_id 
					LIMIT $1 OFFSET $2
				)
			ON CONFLICT (policy_id, cluster_id, compliance_date) DO NOTHING
		`
		selectInsertSQL := fmt.Sprintf(selectInsertSQLTemplate, interval, tableName)
		result, err := pool.Exec(ctx, selectInsertSQL, batchSize, offset)
		if err != nil {
			log.Info("exec failed, retrying", "error", err)
			return false, nil
		}
		insertCount = result.RowsAffected()
		log.Info("from local_status.compliance", "batch", batchSize, "insert", insertCount, "offset", offset)
		return true, nil
	})
	return insertCount, err
}

func syncToLocalComplianceHistoryByPolicyEvent(ctx context.Context, pool *pgxpool.Pool, batchSize int64) (
	totalCount int64, insertedCount int64, err error,
) {
	totalCountSQLTemplate := `
		SELECT COUNT(*) FROM (
			SELECT DISTINCT policy_id, cluster_id FROM event.local_policies
			WHERE created_at BETWEEN CURRENT_DATE - INTERVAL '%d days' AND CURRENT_DATE - INTERVAL '%d day'
		) AS subquery
	`
	totalCountStatement := fmt.Sprintf(totalCountSQLTemplate, dateInterval, dateInterval-1)

	if err := pool.QueryRow(ctx, totalCountStatement).Scan(&totalCount); err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistoryByPolicyEvent(ctx, pool, totalCount, batchSize, offset)
		if err != nil {
			return totalCount, insertedCount, err
		}
		insertedCount += count
	}

	return totalCount, insertedCount, nil
}

func insertToLocalComplianceHistoryByPolicyEvent(ctx context.Context, pool *pgxpool.Pool,
	totalCount, batchSize, offset int64,
) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		var insertError error
		defer func() {
			if e := traceComplianceHistory(ctx, pool,
				fmt.Sprintf("%s/event.local_policies", localComplianceTaskName),
				totalCount, offset, insertCount, startTime, insertError); e != nil {
				log.Info("trace compliance job failed, retrying", "error", e)
			}
		}()
		selectInsertSQLTemplate := `
			INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance_date, compliance,
					compliance_changed_frequency)
			WITH compliance_aggregate AS (
					SELECT cluster_id, policy_id, leaf_hub_name,
							CASE
									WHEN bool_and(compliance = 'compliant') THEN 'compliant'
									ELSE 'non_compliant'
							END::local_status.compliance_type AS aggregated_compliance
					FROM event.local_policies
					WHERE created_at BETWEEN CURRENT_DATE - INTERVAL '%d days' AND CURRENT_DATE - INTERVAL '%d day'
					GROUP BY cluster_id, policy_id, leaf_hub_name
			)
			SELECT policy_id, cluster_id, leaf_hub_name, (CURRENT_DATE - INTERVAL '%d day'), aggregated_compliance,
					(SELECT COUNT(*) FROM (
							SELECT created_at, compliance, 
									LAG(compliance) OVER (PARTITION BY cluster_id, policy_id ORDER BY created_at ASC)
									AS prev_compliance
							FROM event.local_policies lp
							WHERE (lp.created_at BETWEEN CURRENT_DATE - INTERVAL '%d days' AND CURRENT_DATE - INTERVAL '%d day') 
									AND lp.cluster_id = ca.cluster_id AND lp.policy_id = ca.policy_id
							ORDER BY created_at ASC
					) AS subquery WHERE compliance <> prev_compliance) AS compliance_changed_frequency
			FROM compliance_aggregate ca
			ORDER BY cluster_id, policy_id
			LIMIT $1 OFFSET $2
			ON CONFLICT (policy_id, cluster_id, compliance_date)
			DO UPDATE SET
				compliance = EXCLUDED.compliance,
				compliance_changed_frequency = EXCLUDED.compliance_changed_frequency;
			`
		selectInsertStatement := fmt.Sprintf(selectInsertSQLTemplate, dateInterval, dateInterval-1,
			dateInterval, dateInterval, dateInterval-1)

		var result pgconn.CommandTag
		result, insertError = pool.Exec(ctx, selectInsertStatement, batchSize, offset)
		if insertError != nil {
			log.Info("insert failed, retrying", "error", insertError)
			return false, nil
		}
		insertCount = result.RowsAffected()
		log.Info("from event.local_policies", "batch", batchSize, "insert", insertCount, "offset", offset)
		return true, nil
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
	INSERT INTO history.local_compliance_job_log (name, start_at, end_at, total, offsets, inserted, error) 
	VALUES ($1, $2, $3, $4, $5, $6, $7);`, name, start, end, total, offset, inserted, errMessage)

	return err
}
