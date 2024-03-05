package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/monitoring"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var (
	LocalComplianceTaskName = "local-compliance-history"
	startTime               time.Time
	log                     logr.Logger
	timeFormat              = "2006-01-02 15:04:05"
	dateFormat              = "2006-01-02"
	dateInterval            = 1
	simulationCounter       = 1
	counterLock             sync.Mutex
	batchSize               = int64(1000)
	// batchSize = 1000 for now
	// The suitable batchSize for selecting and inserting a lot of records from a table in PostgreSQL depends on
	// several factors such as the size of the table, available memory, network bandwidth, and hardware specifications.
	// However, as a general rule of thumb, a batch size of around 1000 to 5000 records is a good starting point. This
	// size provides a balance between minimizing the number of queries sent to the server while still being efficient
	// and not overloading the system. To determine the optimal batchSize, it may be helpful to test different batch
	// sizes and measure the performance of the queries.
)

func SyncLocalCompliance(ctx context.Context, enableSimulation bool, job gocron.Job) {
	startTime = time.Now()

	interval := dateInterval
	if enableSimulation {
		// When the interval is so small that the previous job has not finished running, the next job has already started.
		// Then the a race condition will arise: the previous and next jobs will using the same view, which will cause the
		// next job to fail. To avoid this, lock the counter so only one goroutine can access the it and each goroutine
		// will get a different simulatorCounter value.
		counterLock.Lock()
		interval = simulationCounter
		simulationCounter++
		counterLock.Unlock()
	}

	var err error
	defer func() {
		if err != nil {
			monitoring.GlobalHubCronJobGaugeVec.WithLabelValues(LocalComplianceTaskName).Set(1)
		} else {
			monitoring.GlobalHubCronJobGaugeVec.WithLabelValues(LocalComplianceTaskName).Set(0)
		}
	}()

	historyDate := startTime.AddDate(0, 0, -interval)
	log = ctrl.Log.WithName(LocalComplianceTaskName).WithValues("history", historyDate.Format(dateFormat))
	log.V(2).Info("start running", "currentRun", job.LastRun().Format(timeFormat))
	conn := database.GetConn()

	err = database.Lock(conn)
	if err != nil {
		retentionLog.Error(err, "failed to run SyncLocalCompliance job")
		return
	}
	defer database.Unlock(conn)

	// insert or update with local_status.compliance
	var statusTotal, statusInsert int64
	statusTotal, statusInsert, err = syncToLocalComplianceHistoryByLocalStatus(ctx, batchSize, interval,
		enableSimulation)
	if err != nil {
		log.Error(err, "sync from local_status.compliance to history.local_compliance failed")
		return
	}
	log.V(2).Info("with local_status.compliance", "totalCount", statusTotal, "insertedCount", statusInsert)

	if enableSimulation {
		return
	}

	// insert or update with event.local_policies
	var eventTotal, eventInsert int64
	eventTotal, eventInsert, err = syncToLocalComplianceHistoryByPolicyEvent(ctx, batchSize)
	if err != nil {
		log.Error(err, "sync from history.local_policies to history.local_compliance failed")
		return
	}
	log.V(2).Info("with event.local_policies", "totalCount", eventTotal, "insertedCount", eventInsert)

	log.V(2).Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
}

func syncToLocalComplianceHistoryByLocalStatus(ctx context.Context, batchSize int64, interval int,
	enableSimulation bool) (totalCount int64, insertedCount int64, err error,
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

	db := database.GetGorm()

	err = db.Exec(fmt.Sprintf(createViewTemplate, viewName, viewName)).Error
	if err != nil {
		return totalCount, insertedCount, err
	}

	err = db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", viewName)).Scan(&totalCount).Error
	if err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistoryByLocalStatus(ctx, viewName, interval, totalCount,
			batchSize, offset, enableSimulation)
		if err != nil {
			return totalCount, insertedCount, err
		}
		insertedCount += count
	}

	// success, drop the materialized view if exists
	err = db.Exec(fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName)).Error
	if err != nil {
		return totalCount, insertedCount, err
	}

	return totalCount, insertedCount, nil
}

func insertToLocalComplianceHistoryByLocalStatus(ctx context.Context, tableName string, interval int,
	totalCount, batchSize, offset int64, enableSimulation bool,
) (int64, error) {
	insertCount := int64(0)
	var err error
	defer func() {
		if e := traceComplianceHistoryLog(ctx,
			fmt.Sprintf("%s/local_status.compliance", LocalComplianceTaskName),
			totalCount, offset, insertCount, startTime, err); e != nil {
			log.Info("trace compliance job failed, retrying", "error", e)
		}
	}()
	err = wait.PollUntilContextTimeout(ctx, 5*time.Second, 10*time.Minute, true,
		func(ctx context.Context) (done bool, err error) {
			selectInsertSQLTemplate := `
			INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance, compliance_date) 
				(
					SELECT policy_id,cluster_id,leaf_hub_name,compliance,(CURRENT_DATE - INTERVAL '%d day') 
					FROM %s 
					ORDER BY policy_id, cluster_id 
					LIMIT %d OFFSET %d
				)
			ON CONFLICT (policy_id, cluster_id, compliance_date) DO NOTHING
		`
			if enableSimulation {
				selectInsertSQLTemplate = `
				do
				$$
				declare
					all_compliances local_status.compliance_type[] := '{"compliant","non_compliant"}';
					compliance_random_index int;
				begin
					SELECT floor(random() * 2 + 1)::int into compliance_random_index;
					INSERT INTO history.local_compliance (policy_id, cluster_id, leaf_hub_name, compliance, compliance_date) 
						(
							SELECT policy_id,cluster_id,leaf_hub_name,all_compliances[compliance_random_index],
							(CURRENT_DATE - INTERVAL '%d day') 
							FROM %s 
							ORDER BY policy_id, cluster_id 
							LIMIT %d OFFSET %d
						)
					ON CONFLICT (policy_id, cluster_id, compliance_date) DO NOTHING;
				end;
				$$;
			`
			}
			db := database.GetGorm()

			selectInsertSQL := fmt.Sprintf(selectInsertSQLTemplate, interval, tableName, batchSize, offset)
			result := db.Exec(selectInsertSQL)
			if result.Error != nil {
				log.Info("exec failed, retrying", "error", result.Error)
				return false, nil
			}
			insertCount = result.RowsAffected
			log.V(2).Info("from local_status.compliance", "batch", batchSize, "insert", insertCount, "offset", offset)
			return true, nil
		})
	return insertCount, err
}

func syncToLocalComplianceHistoryByPolicyEvent(ctx context.Context, batchSize int64) (
	totalCount int64, insertedCount int64, err error,
) {
	totalCountSQLTemplate := `
		SELECT COUNT(1) FROM (
			SELECT DISTINCT policy_id, cluster_id FROM event.local_policies
			WHERE created_at BETWEEN CURRENT_DATE - INTERVAL '%d days' AND CURRENT_DATE - INTERVAL '%d day'
		) AS subquery
	`
	totalCountStatement := fmt.Sprintf(totalCountSQLTemplate, dateInterval, dateInterval-1)

	db := database.GetGorm()
	if err := db.Raw(totalCountStatement).Scan(&totalCount).Error; err != nil {
		return totalCount, insertedCount, err
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := insertToLocalComplianceHistoryByPolicyEvent(ctx, totalCount, batchSize, offset)
		if err != nil {
			return totalCount, insertedCount, err
		}
		insertedCount += count
	}

	return totalCount, insertedCount, nil
}

func insertToLocalComplianceHistoryByPolicyEvent(ctx context.Context, totalCount, batchSize, offset int64,
) (int64, error) {
	insertCount := int64(0)
	err := wait.PollUntilContextTimeout(ctx, 5*time.Second, 30*time.Minute, true,
		func(ctx context.Context) (done bool, err error) {
			var insertError error
			defer func() {
				if e := traceComplianceHistoryLog(ctx,
					fmt.Sprintf("%s/event.local_policies", LocalComplianceTaskName),
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
									WHEN bool_or(compliance = 'non_compliant') THEN 'non_compliant'
									WHEN bool_or(compliance = 'unknown') THEN 'unknown'
									ELSE 'compliant'
							END::local_status.compliance_type AS aggregated_compliance
					FROM 
						event.local_policies lp
					WHERE 
						created_at BETWEEN CURRENT_DATE - INTERVAL '%d days' AND CURRENT_DATE - INTERVAL '%d day'
						AND EXISTS (
							SELECT 1
							FROM status.leaf_hubs lh
							WHERE lh.leaf_hub_name = lp.leaf_hub_name
						)
					GROUP BY cluster_id, policy_id, leaf_hub_name
			)
			SELECT policy_id, cluster_id, leaf_hub_name, (CURRENT_DATE - INTERVAL '%d day'), aggregated_compliance,
					(SELECT COUNT(1) FROM (
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

			db := database.GetGorm()
			result := db.Exec(selectInsertStatement, batchSize, offset)
			insertError = result.Error
			if insertError != nil {
				log.Info("insert failed, retrying", "error", insertError)
				return false, nil
			}
			insertCount = result.RowsAffected
			log.V(2).Info("from event.local_policies", "batch", batchSize, "insert", insertCount, "offset", offset)
			return true, nil
		})
	return insertCount, err
}

func traceComplianceHistoryLog(ctx context.Context, name string, total, offset, inserted int64,
	start time.Time, err error,
) error {
	db := database.GetGorm()
	localComplianceJobLog := &models.LocalComplianceJobLog{
		Name:     name,
		StartAt:  start,
		EndAt:    time.Now(),
		Error:    "none",
		Total:    total,
		Offsets:  offset,
		Inserted: inserted,
	}
	if err != nil {
		localComplianceJobLog.Error = err.Error()
	}
	return db.Create(localComplianceJobLog).Error
}
