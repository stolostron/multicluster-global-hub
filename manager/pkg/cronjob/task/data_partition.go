package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/manager/pkg/metrics"
	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
)

var (
	// The main tasks of this job are:
	// 		1. create partition tables for days in the future, the partition table for the next month is created
	//    2. delete partition tables that are no longer needed, the partition table for the previous 18 month is deleted
	//    3. completely delete the soft deleted records from database after retainedMonths
	retentionTaskName = "data-partition"

	// after the record is marked as deleted, retentionMonth is used to indicate how long it will be retained
	// before it is completely deleted from database
	// retentionMonth  = 18
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
		"history.local_compliance",
	}
	partitionLog = ctrl.Log.WithName(retentionTaskName)
)

func init() {
	metrics.GlobalHubPartitionJobGauge.Set(0)
}

func DataRetention(ctx context.Context, pool *pgxpool.Pool, retention time.Duration, job gocron.Job) {
	currentTime := time.Now()

	var err error
	defer func() {
		if err != nil {
			metrics.GlobalHubPartitionJobGauge.Set(1)
		} else {
			metrics.GlobalHubPartitionJobGauge.Set(0)
		}
	}()

	creationPartitionTime := currentTime.AddDate(0, 1, 0)
	deletionPartitionTime := currentTime.Add(-retention).AddDate(0, -1, 0)
	for _, tableName := range partitionTables {
		err = updatePartitionTables(tableName, creationPartitionTime, deletionPartitionTime)
		if e := traceDataRetentionLog(tableName, currentTime, err, true); e != nil {
			partitionLog.Error(e, "failed to trace data retention log")
		}
		if err != nil {
			partitionLog.Error(err, "failed to update partition tables")
			return
		}
	}

	// delete the soft deleted records from database
	for _, tableName := range retentionTables {
		err = deleteExpiredRecords(tableName, deletionPartitionTime)
		if e := traceDataRetentionLog(tableName, currentTime, err, false); e != nil {
			partitionLog.Error(e, "failed to trace data retention log")
		}
		if err != nil {
			partitionLog.Error(err, "failed to delete soft deleted records")
			return
		}
	}

	partitionLog.Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
}

func updatePartitionTables(tableName string, createTime, deleteTime time.Time) error {
	db := database.GetGorm()

	// create the partition tables for the next month
	startTime := time.Date(createTime.Year(), createTime.Month(), 1, 0, 0, 0, 0, createTime.Location())
	endTime := startTime.AddDate(0, 1, 0)
	createPartitionTableName := fmt.Sprintf("%s_%s", tableName, startTime.Format(partitionDateFormat))

	creationSql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		createPartitionTableName, tableName, startTime.Format(dateFormat), endTime.Format(dateFormat))
	if result := db.Exec(creationSql); result.Error != nil {
		return fmt.Errorf("failed to create partition table %s: %w", tableName, result.Error)
	}
	partitionLog.Info("create partition table", "table", createPartitionTableName, "start", startTime.Format(dateFormat),
		"end", endTime.Format(dateFormat))

	// delete the partition tables that are expired
	deletePartitionTableName := fmt.Sprintf("%s_%s", tableName, deleteTime.Format(partitionDateFormat))
	deletionSql := fmt.Sprintf("DROP TABLE IF EXISTS %s", deletePartitionTableName)
	if result := db.Exec(deletionSql); result.Error != nil {
		return fmt.Errorf("failed to delete partition table %s: %w", tableName, result.Error)
	}
	partitionLog.Info("delete partition table", "table", deletePartitionTableName)
	return nil
}

func deleteExpiredRecords(tableName string, deleteTime time.Time) error {
	minTime := deleteTime.AddDate(0, 1, 0)
	minDate := time.Date(minTime.Year(), minTime.Month(), 1, 0, 0, 0, 0, minTime.Location())
	sql := fmt.Sprintf("DELETE FROM %s WHERE deleted_at < '%s'", tableName, minDate.Format(dateFormat))
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to delete records before %s from %s: %w",
			minDate.Format(dateFormat), tableName, result.Error)
	}
	partitionLog.Info("delete records", "table", tableName, "before", minDate.Format(dateFormat))
	return nil
}

func traceDataRetentionLog(tableName string, startTime time.Time, err error, partition bool) error {
	db := database.GetGorm()
	dataRetentionLog := &models.DataRetentionJobLog{
		Name:    tableName,
		StartAt: startTime,
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
		if minDeletionTime, err := getMinDeletionTime(tableName); err != nil && !minDeletionTime.IsZero() {
			dataRetentionLog.MinDeletion = minDeletionTime
		}
	}
	result := db.Create(dataRetentionLog)
	return result.Error
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
		partitionLog.Info("no partition table found", "table", tableName)
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
