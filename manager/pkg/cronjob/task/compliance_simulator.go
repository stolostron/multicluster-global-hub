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
	viewName string
	counter  = 1
)

func simulateLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool, batchSize int64, job gocron.Job) {
	simulateDate := startTime.AddDate(0, 0, -counter)
	log = ctrl.Log.WithName(localComplianceTaskName).WithValues("simulateDate", simulateDate.Format(dateFormat))
	log.Info("start simulating", "currentRun", job.LastRun().Format(timeFormat))

	totalCount, insertedCount := int64(0), int64(0)
	// create materialized view
	viewName = fmt.Sprintf("local_status.compliance_view_%s",
		time.Now().AddDate(0, 0, -counter).Format("2006_01_02"))
	createViewTemplate := `
		CREATE MATERIALIZED VIEW IF NOT EXISTS %s AS 
		SELECT id,cluster_id,compliance 
		FROM local_status.compliance;
		CREATE INDEX IF NOT EXISTS idx_local_compliance_view ON %s (id, cluster_id);
	`
	_, err := pool.Exec(ctx, fmt.Sprintf(createViewTemplate, viewName, viewName))
	if err != nil {
		log.Error(err, "create materialized view failed")
		return
	}
	// refresh the materialized view
	// _, err = conn.Exec(ctx, "REFRESH MATERIALIZED VIEW local_compliance_mv")
	// if err != nil {
	// 	return totalCount, syncedCount, err
	// }

	err = pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", viewName)).Scan(&totalCount)
	if err != nil {
		log.Error(err, "get total count failed")
		return
	}

	for offset := int64(0); offset < totalCount; offset += batchSize {
		count, err := simulateToLocalComplianceHistory(ctx, pool, totalCount, batchSize, offset)
		if err != nil {
			log.Error(err, "simulateToLocalComplianceHistory failed")
			return
		}
		insertedCount += count
	}

	// success, drop the materialized view if exists
	_, err = pool.Exec(ctx, fmt.Sprintf("DROP MATERIALIZED VIEW IF EXISTS %s", viewName))
	if err != nil {
		log.Error(err, "drop materialized view failed")
		return
	}
	counter = counter + 1

	log.Info("finish simulating", "totalCount", totalCount, "insertedCount", insertedCount, "batchSize", batchSize,
		"nextRun", job.NextRun().Format(timeFormat))
}

func simulateToLocalComplianceHistory(ctx context.Context, pool *pgxpool.Pool,
	totalCount, batchSize, offset int64,
) (int64, error) {
	// retry until success, use timeout context to avoid long running
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	insertCount := int64(0)
	err := wait.PollUntilWithContext(timeoutCtx, 5*time.Second, func(ctx context.Context) (done bool, err error) {
		selectInsertSQLTemplate := `
			INSERT INTO history.local_compliance (id, cluster_id, compliance, compliance_date) 
			SELECT id,cluster_id,compliance,(CURRENT_DATE - INTERVAL '%d day') 
			FROM %s 
			ORDER BY id, cluster_id
			LIMIT $1 
			OFFSET $2
		`
		selectInsertSQL := fmt.Sprintf(selectInsertSQLTemplate, counter, viewName)
		result, insertError := pool.Exec(ctx, selectInsertSQL, batchSize, offset)
		if insertError != nil {
			log.Info("exec failed, retrying", "error", insertError)
			return false, nil
		}
		insertCount = result.RowsAffected()
		log.Info("insert success", "inserted", insertCount, "offset", offset, "batchSize", batchSize)
		return true, nil
	})
	return insertCount, err
}
