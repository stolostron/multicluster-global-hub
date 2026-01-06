package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/processes/hubmanagement"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
	"github.com/stolostron/multicluster-global-hub/pkg/logger"
)

var (
	// The main tasks of this job are:
	// 1. ensure current month partition exists (handles operator restart scenarios)
	// 2. create partition tables for the next month
	// 3. delete partition tables that are no longer needed (beyond retention period)
	// 4. completely delete the soft deleted records from database after retainedMonths
	RetentionTaskName = "data-retention"

	// after the record is marked as deleted, retentionMonth is used to indicate how long it will be retained
	// before it is completely deleted from database
	// retentionMonth  = 18
	RetentionTables = []string{
		"status.managed_clusters",
		"status.leaf_hubs",
		"local_spec.policies",
	}

	// partition by month
	PartitionDateFormat = "2006_01"
	// the following data tables will generate records over time, so it is necessary to split them into small tables to
	// achieve better scanning and writing performance.
	PartitionTables = []string{
		"event.local_policies",
		"event.local_root_policies",
		"history.local_compliance",
		"event.managed_clusters",
	}
	retentionLog = logger.ZapLogger(RetentionTaskName)

	// dataRetentionMu prevents concurrent execution of DataRetention job
	dataRetentionMu sync.Mutex
)

func DataRetention(ctx context.Context, retentionMonth int, job gocron.Job) {
	dataRetentionMu.Lock()
	defer dataRetentionMu.Unlock()

	now := time.Now()
	currentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	var err error
	db := database.GetGorm()
	conn := database.GetConn()
	err = database.Lock(conn)
	if err != nil {
		retentionLog.Error(err, "failed to run data retention")
		return
	}
	defer database.Unlock(conn)

	defer func() {
		if err != nil {
			GlobalHubCronJobGaugeVec.WithLabelValues(RetentionTaskName).Set(1)
		} else {
			GlobalHubCronJobGaugeVec.WithLabelValues(RetentionTaskName).Set(0)
		}
	}()

	// Ensure current month partition exists to handle operator restart scenario
	// This fixes the issue where operator restart between the 28th and month-end
	// could result in missing current month partitions after month rollover
	for _, tableName := range PartitionTables {
		err = ensurePartitionExists(tableName, currentMonth)
		if err != nil {
			retentionLog.Error(err, "failed to ensure current month partition exists")
			return
		}
	}

	// Create next month partition and delete expired partitions
	createMonth := currentMonth.AddDate(0, 1, 0)
	deleteMonth := currentMonth.AddDate(0, -(retentionMonth + 1), 0)
	for _, tableName := range PartitionTables {
		err = updatePartitionTables(tableName, createMonth, deleteMonth)
		if e := traceDataRetentionLog(tableName, currentMonth, err, true); e != nil {
			retentionLog.Error(e, "failed to trace data retention log")
		}
		if err != nil {
			retentionLog.Error(err, "failed to update partition tables")
			return
		}
	}

	// delete the soft deleted records from database
	minTime := currentMonth.AddDate(0, -retentionMonth, 0)
	for _, tableName := range RetentionTables {
		err = deleteExpiredRecords(tableName, minTime)
		if e := traceDataRetentionLog(tableName, currentMonth, err, false); e != nil {
			retentionLog.Error(e, "failed to trace data retention log")
		}
		if err != nil {
			retentionLog.Error(err, "failed to delete soft deleted records")
			return
		}
	}
	err = db.Where("last_timestamp < ? AND status = ?", minTime, hubmanagement.HubInactive).
		Delete(&models.LeafHubHeartbeat{}).Error
	if err != nil {
		retentionLog.Error(err, "failed to delete the expired leaf hub heartbeat")
		return
	}
	retentionLog.Info("finish running", "nextRun", job.NextRun().Format(TimeFormat))
}

// ensurePartitionExists ensures a partition table exists for the given month
// This is idempotent and safe to call multiple times
func ensurePartitionExists(tableName string, createTime time.Time) error {
	db := database.GetGorm()

	startTime := time.Date(createTime.Year(), createTime.Month(), 1, 0, 0, 0, 0, createTime.Location())
	endTime := startTime.AddDate(0, 1, 0)
	createPartitionTableName := fmt.Sprintf("%s_%s", tableName, startTime.Format(PartitionDateFormat))

	creationSql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		createPartitionTableName, tableName, startTime.Format(DateFormat), endTime.Format(DateFormat))
	if result := db.Exec(creationSql); result.Error != nil {
		return fmt.Errorf("failed to ensure partition table %s exists: %w", createPartitionTableName, result.Error)
	}
	retentionLog.Info("create partition table", "table", createPartitionTableName, "start", startTime.Format(DateFormat),
		"end", endTime.Format(DateFormat))
	return nil
}

func updatePartitionTables(tableName string, createTime, deleteTime time.Time) error {
	db := database.GetGorm()

	// create the partition tables for the next month
	err := ensurePartitionExists(tableName, createTime)
	if err != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, err)
	}

	// delete the partition tables that are expired
	deletePartitionTableName := fmt.Sprintf("%s_%s", tableName, deleteTime.Format(PartitionDateFormat))
	deletionSql := fmt.Sprintf("DROP TABLE IF EXISTS %s", deletePartitionTableName)
	if result := db.Exec(deletionSql); result.Error != nil {
		return fmt.Errorf("failed to delete partition table %s: %w", tableName, result.Error)
	}
	retentionLog.Info("delete partition table", "table", deletePartitionTableName)
	return nil
}

func deleteExpiredRecords(tableName string, minDate time.Time) error {
	sql := fmt.Sprintf("DELETE FROM %s WHERE deleted_at < '%s'", tableName, minDate.Format(DateFormat))
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to delete records before %s from %s: %w",
			minDate.Format(DateFormat), tableName, result.Error)
	}
	retentionLog.Info("delete records", "table", tableName, "before", minDate.Format(DateFormat))
	return nil
}

func traceDataRetentionLog(tableName string, startTime time.Time, err error, partition bool) error {
	db := database.GetGorm()
	dataRetentionLog := &models.DataRetentionJobLog{
		Name:    tableName,
		StartAt: startTime,
		EndAt:   time.Now(),
		Error:   "none",
	}
	if err != nil {
		dataRetentionLog.Error = err.Error()
	}

	if partition {
		minPartition, maxPartition, err := getMinMaxPartitions(tableName)
		if err != nil {
			return err
		}
		dataRetentionLog.MinPartition = minPartition
		dataRetentionLog.MaxPartition = maxPartition
	} else {
		if minDeletionTime, err := getMinDeletionTime(tableName); err == nil && !minDeletionTime.IsZero() {
			dataRetentionLog.MinDeletion = minDeletionTime
		}
	}
	return db.Create(dataRetentionLog).Error
}

func getMinMaxPartitions(tableName string) (string, string, error) {
	db := database.GetGorm()

	schemaTable := strings.Split(tableName, ".")
	if len(schemaTable) != 2 {
		return "", "", fmt.Errorf("invalid table name: %s", tableName)
	}
	sql := fmt.Sprintf(`
		SELECT
			nmsp_child.nspname AS schema_name,
			child.relname AS table_name
		FROM
			pg_inherits
			JOIN pg_class parent ON pg_inherits.inhparent = parent.oid
			JOIN pg_class child ON pg_inherits.inhrelid = child.oid
			JOIN pg_namespace nmsp_parent ON nmsp_parent.oid = parent.relnamespace
			JOIN pg_namespace nmsp_child ON nmsp_child.oid = child.relnamespace
		WHERE
			nmsp_parent.nspname='%s'
			AND parent.relname='%s'
		ORDER BY
			child.relname ASC;`,
		schemaTable[0], schemaTable[1])

	var tables []models.Table
	result := db.Raw(sql).Find(&tables)
	if result.Error != nil {
		return "", "", fmt.Errorf("failed to get min/max partition table: %w", result.Error)
	}
	if len(tables) < 1 {
		retentionLog.Info("no partition table found", "table", tableName)
		return "", "", nil
	}
	return tables[0].Table, tables[len(tables)-1].Table, nil
}

func getMinDeletionTime(tableName string) (time.Time, error) {
	db := database.GetGorm()
	minDeletion := &models.Time{}

	result := db.Raw(fmt.Sprintf("SELECT MIN(deleted_at) as time FROM %s", tableName)).Find(minDeletion)
	if result.Error != nil {
		return minDeletion.Time, fmt.Errorf("failed to get min deletion time: %w", result.Error)
	}
	return minDeletion.Time, nil
}
