package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// The main tasks of this job are:
	// 		1. create partition tables for days in the future, the partition table for the next month is created
	//    2. delete partition tables that are no longer needed, the partition table for the previous 18 month is deleted
	//    3. completely delete the soft deleted records from database after retainedMonths
	retentionTaskName = "data-retention"

	// after the record is marked as deleted, retentionMonth is used to indicate how long it will be retained
	// before it is completely deleted from database
	retentionMonth  = 18
	retentionTables = []string{
		"status.managed_clusters",
		"status.leaf_hubs",
		"local_spec.policies",
	}

	// partition by month
	partitionDateFormat = "2006_01"
	// the following data tables will generate records over time, so it is necessary to split them into small tables to
	// achieve better scanning and writing performance.
	partitionTables = []string{
		"event.local_policies",
		"event.local_root_policies",
	}
)

func DataRetention(ctx context.Context, pool *pgxpool.Pool, job gocron.Job) {
	currentTime := time.Now()
	log = ctrl.Log.WithName(retentionTaskName)

	var err error
	defer func() {
		if e := traceDataRetentionJob(retentionTaskName, currentTime, err); e != nil {
			log.Error(e, "trace data retention failed")
		}
	}()

	nextMonth := currentTime.AddDate(0, 1, 0)
	startDate := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, nextMonth.Location())
	endDate := startDate.AddDate(0, 1, 0)

	// create the next month's partition table
	for _, tableName := range partitionTables {
		if err = createPartitionTable(tableName, startDate, endDate); err != nil {
			log.Error(err, "failed to create partition table")
			return
		}
	}

	// delete the retainedMonths ago's partition table
	for _, tableName := range partitionTables {
		if err = deletePartitionTable(tableName, currentTime.AddDate(0, -retentionMonth, 0)); err != nil {
			log.Error(err, "failed to delete partition table")
			return
		}
	}

	// delete the soft deleted records from database
	retentionDateStr := currentTime.AddDate(0, -retentionMonth, 0).Format(dateFormat)
	for _, tableName := range retentionTables {
		if err = deleteRetentionRecords(tableName, retentionDateStr); err != nil {
			log.Error(err, "failed to delete soft deleted records")
			return
		}
	}

	log.Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
}

func createPartitionTable(tableName string, startTime, endTime time.Time) error {
	partitionTableName := fmt.Sprintf("%s_%s", tableName, startTime.Format(partitionDateFormat))
	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		partitionTableName, tableName, startTime.Format(dateFormat), endTime.Format(dateFormat))
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, result.Error)
	}
	return nil
}

func deletePartitionTable(tableName string, dateTime time.Time) error {
	partitionTable := fmt.Sprintf("%s_%s", tableName, dateTime.Format(partitionDateFormat))
	sql := fmt.Sprintf("DROP TABLE IF EXISTS %s", partitionTable)
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to delete partition table %s: %w", tableName, result.Error)
	}
	return nil
}

func deleteRetentionRecords(tableName string, retentionDateStr string) error {
	sql := fmt.Sprintf("DELETE FROM %s WHERE deleted_at < '%s'", tableName, retentionDateStr)
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to delete records before %s from %s: %w", retentionDateStr, tableName, result.Error)
	}
	return nil
}

func traceDataRetentionJob(name string, start time.Time, err error) error {
	end := time.Now()
	errMessage := "none"
	if err != nil {
		errMessage = err.Error()
	}
	db := database.GetGorm()
	result := db.Exec(`INSERT INTO event.data_retention_job_log (name, start_at, end_at, error) 
	VALUES (?, ?, ?, ?);`, name, start, end, errMessage)
	return result.Error
}
