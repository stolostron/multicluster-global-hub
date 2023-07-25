package task

import (
	"context"
	"fmt"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/jackc/pgx/v4/pgxpool"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-global-hub/pkg/database"
	"github.com/stolostron/multicluster-global-hub/pkg/database/models"
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

	creationTime := currentTime.AddDate(0, 1, 0)
	deletionTime := currentTime.AddDate(0, -retentionMonth, 0)
	for _, tableName := range partitionTables {
		err := updatePartitionTables(tableName, creationTime, deletionTime)
		traceDataRetentionLog(tableName, currentTime, err, true)
		if err != nil {
			log.Error(err, "failed to update partition tables")
			return
		}
	}

	// delete the soft deleted records from database
	for _, tableName := range retentionTables {
		err := deleteExpiredRecords(tableName, deletionTime)
		traceDataRetentionLog(tableName, currentTime, err, false)
		if err != nil {
			log.Error(err, "failed to delete soft deleted records")
			return
		}
	}

	log.Info("finish running", "nextRun", job.NextRun().Format(timeFormat))
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

	// delete the partition tables that are expired
	deletePartitionTableName := fmt.Sprintf("%s_%s", tableName, deleteTime.Format(partitionDateFormat))
	deletionSql := fmt.Sprintf("DROP TABLE IF EXISTS %s", deletePartitionTableName)
	if result := db.Exec(deletionSql); result.Error != nil {
		return fmt.Errorf("failed to delete partition table %s: %w", tableName, result.Error)
	}
	return nil
}

func deleteExpiredRecords(tableName string, deleteTime time.Time) error {
	minDateStr := deleteTime.Format(dateFormat)
	sql := fmt.Sprintf("DELETE FROM %s WHERE deleted_at < '%s'", tableName, minDateStr)
	db := database.GetGorm()
	if result := db.Exec(sql); result.Error != nil {
		return fmt.Errorf("failed to delete records before %s from %s: %w",
			minDateStr, tableName, result.Error)
	}
	return nil
}

func traceDataRetentionLog(tableName string, currentTime time.Time, err error, partition bool) error {
	db := database.GetGorm()
	dataRetentionLog := &models.DataRetentionJobLog{
		Name:    tableName,
		StartAt: currentTime,
		Error:   "none",
	}
	if err != nil {
		dataRetentionLog.Error = err.Error()
	}

	if partition {
		minPartition, err := getBoundaryPartitionTable(tableName, Asc)
		if err != nil {
			return fmt.Errorf("failed to get min partition table: %w", err)
		}
		maxPartition, err := getBoundaryPartitionTable(tableName, Desc)
		if err != nil {
			return fmt.Errorf("failed to get max partition table: %w", err)
		}
		dataRetentionLog.MinPartition = minPartition
		dataRetentionLog.MaxPartition = maxPartition
	} else {
		if minDeletionTime, err := getMinDeletionTime(tableName); err != nil && minDeletionTime.IsZero() {
			dataRetentionLog.MinDeletion = minDeletionTime
		}
	}
	result := db.Create(dataRetentionLog)
	return result.Error
}

type Order string

const (
	Asc  Order = "ASC"
	Desc Order = "DESC"
)

func getBoundaryPartitionTable(tableName string, order Order) (string, error) {
	db := database.GetGorm()
	sql := fmt.Sprintf(`
		SELECT
			table_schema || '.' || table_name AS "table_schema.table_name"
		FROM
			information_schema.tables
		WHERE
			table_schema || '.' || table_name LIKE '%s\%%'
		ORDER BY
			table_name %s
		LIMIT 1;`,
		tableName, order)
	var boundaryPartitionTable string
	result := db.Raw(sql).Find(&boundaryPartitionTable)
	if result.Error != nil {
		return "", fmt.Errorf("failed to get boundary partition table: %w", result.Error)
	}
	return boundaryPartitionTable, nil
}

type minDeletion struct {
	DeletedAt time.Time `gorm:"column:deleted_at;default:(-)"`
}

func getMinDeletionTime(tableName string) (time.Time, error) {
	db := database.GetGorm()
	minDeletion := &minDeletion{}
	result := db.Raw(fmt.Sprintf("SELECT MIN(deleted_at) as deleted_at FROM %s", tableName)).Find(minDeletion)
	if result.Error != nil {
		return minDeletion.DeletedAt, fmt.Errorf("failed to get min deletion time: %w", result.Error)
	}
	return minDeletion.DeletedAt, nil
}
